package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/m2p/clm/pkg/config"
)

// DB wraps the sql.DB connection.
type DB struct {
	*sql.DB
}

// NewDB creates a new PostgreSQL connection.
func NewDB(cfg *config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &DB{db}, nil
}

// WithTenantContext executes a query or transaction scoped strictly to the current tenant session
// using transaction-local set_config('app.current_tenant_id', $1, true) with parameterized execution
// to guarantee zero connection-pool leakage across tenant transactions.
func (db *DB) WithTenantContext(ctx context.Context, tenantID uuid.UUID, fn func(tx *sql.Tx) error) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("tenant isolation violation: tenantID cannot be nil")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting tenant-scoped transaction: %w", err)
	}
	defer tx.Rollback()

	// Parameterized transaction-local set_config: is_local=true ensures context clears on commit or rollback
	var setVal string
	err = tx.QueryRowContext(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tenantID.String()).Scan(&setVal)
	if err != nil {
		return fmt.Errorf("setting parameterized transaction-local tenant context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
