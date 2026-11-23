package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
)

//go:embed *.sql
var migrationFiles embed.FS

const migrationLockID int64 = 0x434c4d4d494752

var migrationNamePattern = regexp.MustCompile(`^([0-9]{3})_[a-z0-9_]+\.sql$`)

type Migration struct {
	Version  string
	Filename string
	Checksum string
	Contents string
}

type Result struct {
	Applied   []string
	Skipped   []string
	Baselined []string
}

func List() ([]Migration, error) {
	names, err := fs.Glob(migrationFiles, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("listing embedded migrations: %w", err)
	}

	migrations := make([]Migration, 0, len(names))
	for _, name := range names {
		match := migrationNamePattern.FindStringSubmatch(name)
		if match == nil || match[1] == "000" {
			continue
		}
		contents, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading migration %s: %w", name, err)
		}
		sum := sha256.Sum256(contents)
		migrations = append(migrations, Migration{
			Version:  match[1],
			Filename: name,
			Checksum: hex.EncodeToString(sum[:]),
			Contents: string(contents),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	for i, migration := range migrations {
		expected := fmt.Sprintf("%03d", i+1)
		if migration.Version != expected {
			return nil, fmt.Errorf("migration sequence is incomplete: expected %s, found %s", expected, migration.Version)
		}
	}
	if len(migrations) == 0 {
		return nil, errors.New("no application migrations were embedded")
	}
	return migrations, nil
}

func Apply(ctx context.Context, db *sql.DB, baselineLegacy bool) (Result, error) {
	var result Result
	if db == nil {
		return result, errors.New("migration runner requires a database")
	}
	migrations, err := List()
	if err != nil {
		return result, err
	}

	var historyExists, tenantSchemaExists bool
	if err := db.QueryRowContext(ctx, `
		SELECT to_regclass('public.clm_schema_migrations') IS NOT NULL,
		       to_regclass('public.tenants') IS NOT NULL
	`).Scan(&historyExists, &tenantSchemaExists); err != nil {
		return result, fmt.Errorf("checking migration state: %w", err)
	}

	appliedCount := int64(0)
	if historyExists {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM public.clm_schema_migrations`).Scan(&appliedCount); err != nil {
			return result, fmt.Errorf("reading migration history: %w", err)
		}
	}

	needsBaseline := tenantSchemaExists && appliedCount == 0
	if needsBaseline && !baselineLegacy {
		return result, errors.New("database has an existing CLM schema without migration history; validate it and rerun with CLM_BASELINE_LEGACY=true")
	}
	if baselineLegacy && !needsBaseline {
		return result, errors.New("CLM_BASELINE_LEGACY is only valid for an existing schema with empty migration history")
	}

	if err := ensureHistoryTable(ctx, db); err != nil {
		return result, err
	}
	if needsBaseline {
		result.Baselined, err = baseline(ctx, db, migrations)
		if err != nil {
			return result, err
		}
	}

	for _, migration := range migrations {
		applied, err := applyOne(ctx, db, migration)
		if err != nil {
			return result, err
		}
		if applied {
			result.Applied = append(result.Applied, migration.Filename)
		} else {
			result.Skipped = append(result.Skipped, migration.Filename)
		}
	}
	return result, nil
}

func ensureHistoryTable(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting migration-history transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("locking migration history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS public.clm_schema_migrations (
			version CHAR(3) PRIMARY KEY,
			filename TEXT NOT NULL,
			checksum CHAR(64) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("creating migration history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration-history transaction: %w", err)
	}
	return nil
}

func baseline(ctx context.Context, db *sql.DB, migrations []Migration) ([]string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting legacy baseline transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return nil, fmt.Errorf("locking legacy baseline: %w", err)
	}

	var alreadyRecorded int64
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM public.clm_schema_migrations`).Scan(&alreadyRecorded); err != nil {
		return nil, fmt.Errorf("checking legacy baseline history: %w", err)
	}
	if alreadyRecorded != 0 {
		return nil, errors.New("legacy baseline refused because migration history is no longer empty")
	}
	if err := validateLegacySchema(ctx, tx); err != nil {
		return nil, err
	}

	baselined := make([]string, 0, 4)
	for _, migration := range migrations {
		if migration.Version > "004" {
			break
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO public.clm_schema_migrations (version, filename, checksum)
			VALUES ($1, $2, $3)
		`, migration.Version, migration.Filename, migration.Checksum); err != nil {
			return nil, fmt.Errorf("recording legacy baseline %s: %w", migration.Filename, err)
		}
		baselined = append(baselined, migration.Filename)
	}
	if len(baselined) != 4 {
		return nil, fmt.Errorf("legacy baseline requires migrations 001-004; recorded %d", len(baselined))
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing legacy baseline: %w", err)
	}
	return baselined, nil
}

func validateLegacySchema(ctx context.Context, tx *sql.Tx) error {
	var valid bool
	err := tx.QueryRowContext(ctx, `
		SELECT to_regclass('public.tenants') IS NOT NULL
		   AND to_regclass('public.users') IS NOT NULL
		   AND to_regclass('public.satellites') IS NOT NULL
		   AND to_regclass('public.satellite_commands') IS NOT NULL
		   AND to_regclass('public.control_plane_audit_events') IS NOT NULL
		   AND to_regclass('public.cloud_accounts') IS NOT NULL
		   AND EXISTS (
		       SELECT 1 FROM information_schema.columns
		       WHERE table_schema = 'public'
		         AND table_name = 'satellite_commands'
		         AND column_name = 'lease_token'
		   )
		   AND EXISTS (
		       SELECT 1 FROM information_schema.columns
		       WHERE table_schema = 'public'
		         AND table_name = 'satellite_commands'
		         AND column_name = 'execution_generation'
		   )
	`).Scan(&valid)
	if err != nil {
		return fmt.Errorf("validating legacy database schema: %w", err)
	}
	if !valid {
		return errors.New("legacy baseline refused: required schema objects from migrations 001-004 are missing")
	}
	return nil
}

func applyOne(ctx context.Context, db *sql.DB, migration Migration) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("starting migration %s: %w", migration.Filename, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return false, fmt.Errorf("locking migration %s: %w", migration.Filename, err)
	}

	var filename, checksum string
	err = tx.QueryRowContext(ctx, `
		SELECT filename, checksum
		FROM public.clm_schema_migrations
		WHERE version = $1
	`, migration.Version).Scan(&filename, &checksum)
	switch {
	case err == nil:
		if filename != migration.Filename || checksum != migration.Checksum {
			return false, fmt.Errorf("applied migration %s differs from the embedded source; refusing to continue", migration.Version)
		}
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("releasing migration %s transaction: %w", migration.Filename, err)
		}
		return false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("checking migration %s history: %w", migration.Filename, err)
	}

	if _, err := tx.ExecContext(ctx, migration.Contents); err != nil {
		return false, fmt.Errorf("applying migration %s: %w", migration.Filename, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO public.clm_schema_migrations (version, filename, checksum)
		VALUES ($1, $2, $3)
	`, migration.Version, migration.Filename, migration.Checksum); err != nil {
		return false, fmt.Errorf("recording migration %s: %w", migration.Filename, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing migration %s: %w", migration.Filename, err)
	}
	return true, nil
}
