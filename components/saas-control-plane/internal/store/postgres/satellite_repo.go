package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
	"github.com/m2p/clm/pkg/satelliteauth"
)

var (
	ErrSatelliteNotFound   = fmt.Errorf("satellite: %w", core.ErrNotFound)
	ErrSatelliteAuth       = errors.New("satellite authentication failed")
	ErrInvalidLease        = errors.New("invalid or expired command lease")
	ErrInvalidTransition   = errors.New("invalid command state transition")
	ErrIdempotencyConflict = errors.New("idempotency key is already bound to a different command")
)

type SatelliteRepository struct{ db *DB }

func NewSatelliteRepository(db *DB) *SatelliteRepository { return &SatelliteRepository{db: db} }

func (r *SatelliteRepository) Create(ctx context.Context, sat *models.Satellite) error {
	query := `INSERT INTO satellites (id, tenant_id, name, type, status, connector_mode, version, cloud, region, secrets_backend, credential_hash, credential_generation, credential_created_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13,$14,$15)`
	_, err := r.db.ExecContext(ctx, query, sat.ID, sat.TenantID, sat.Name, sat.Type, sat.Status, sat.ConnectorMode,
		sat.Version, sat.Cloud, sat.Region, sat.SecretsBackend, sat.CredentialHash, sat.CredentialGeneration,
		sat.CredentialCreatedAt, sat.CreatedAt, sat.UpdatedAt)
	return err
}

const satelliteColumns = `id, tenant_id, name, type, status, connector_mode, COALESCE(version,''), last_heartbeat,
	COALESCE(ip_address,''), COALESCE(cloud,''), COALESCE(region,''), COALESCE(secrets_backend,''), agent_count,
	cert_count, credential_generation, credential_created_at, credential_last_used_at, credential_revoked_at,
	created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanSatellite(row rowScanner) (*models.Satellite, error) {
	s := &models.Satellite{}
	err := row.Scan(&s.ID, &s.TenantID, &s.Name, &s.Type, &s.Status, &s.ConnectorMode, &s.Version,
		&s.LastHeartbeat, &s.IPAddress, &s.Cloud, &s.Region, &s.SecretsBackend, &s.AgentCount, &s.CertCount,
		&s.CredentialGeneration, &s.CredentialCreatedAt, &s.CredentialLastUsedAt, &s.CredentialRevokedAt,
		&s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SatelliteRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Satellite, error) {
	s, err := scanSatellite(r.db.QueryRowContext(ctx, `SELECT `+satelliteColumns+` FROM satellites WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSatelliteNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SatelliteRepository) GetByIDForTenant(ctx context.Context, tenantID, id uuid.UUID) (*models.Satellite, error) {
	s, err := scanSatellite(r.db.QueryRowContext(ctx, `SELECT `+satelliteColumns+` FROM satellites WHERE tenant_id=$1 AND id=$2`, tenantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSatelliteNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SatelliteRepository) list(ctx context.Context, query string, args ...any) ([]*models.Satellite, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*models.Satellite
	for rows.Next() {
		s, err := scanSatellite(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SatelliteRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*models.Satellite, error) {
	return r.list(ctx, `SELECT `+satelliteColumns+` FROM satellites WHERE tenant_id=$1 ORDER BY name`, tenantID)
}
func (r *SatelliteRepository) ListAll(ctx context.Context) ([]*models.Satellite, error) {
	return r.list(ctx, `SELECT `+satelliteColumns+` FROM satellites ORDER BY name`)
}
func (r *SatelliteRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM satellites WHERE id=$1`, id)
	return err
}
func (r *SatelliteRepository) DeleteForTenant(ctx context.Context, tenantID, id uuid.UUID) error {
	return requireAffected(r.db.ExecContext(ctx, `DELETE FROM satellites WHERE tenant_id=$1 AND id=$2`, tenantID, id))
}
func (r *SatelliteRepository) Update(ctx context.Context, sat *models.Satellite) error {
	_, err := r.db.ExecContext(ctx, `UPDATE satellites SET status=$1,version=$2,last_heartbeat=$3,agent_count=$4,cert_count=$5,updated_at=$6 WHERE id=$7`,
		sat.Status, sat.Version, sat.LastHeartbeat, sat.AgentCount, sat.CertCount, sat.UpdatedAt, sat.ID)
	return err
}
func (r *SatelliteRepository) UpdateForTenant(ctx context.Context, tenantID uuid.UUID, sat *models.Satellite) error {
	return requireAffected(r.db.ExecContext(ctx, `UPDATE satellites SET name=$1,cloud=$2,region=$3,status=$4,version=$5,last_heartbeat=$6,agent_count=$7,cert_count=$8,updated_at=$9 WHERE tenant_id=$10 AND id=$11`,
		sat.Name, sat.Cloud, sat.Region, sat.Status, sat.Version, sat.LastHeartbeat, sat.AgentCount, sat.CertCount, sat.UpdatedAt, tenantID, sat.ID))
}

func requireAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSatelliteNotFound
	}
	return nil
}

func (r *SatelliteRepository) SetCredentialForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID, encodedHash string) error {
	now := time.Now().UTC()
	return requireAffected(r.db.ExecContext(ctx, `UPDATE satellites SET credential_hash=$1,credential_generation=credential_generation+1,credential_created_at=$2,credential_last_used_at=NULL,credential_revoked_at=NULL,updated_at=$2 WHERE tenant_id=$3 AND id=$4`, encodedHash, now, tenantID, satelliteID))
}
func (r *SatelliteRepository) RevokeCredentialForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID) error {
	now := time.Now().UTC()
	return requireAffected(r.db.ExecContext(ctx, `UPDATE satellites SET credential_revoked_at=$1,updated_at=$1 WHERE tenant_id=$2 AND id=$3`, now, tenantID, satelliteID))
}
func (r *SatelliteRepository) AuthenticateSatellite(ctx context.Context, satelliteID uuid.UUID, plaintext string) (*models.SatelliteIdentity, error) {
	var tenantID uuid.UUID
	var encoded string
	var revoked *time.Time
	err := r.db.QueryRowContext(ctx, `SELECT tenant_id,COALESCE(credential_hash,''),credential_revoked_at FROM satellites WHERE id=$1`, satelliteID).Scan(&tenantID, &encoded, &revoked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if !satelliteauth.Verify(encoded, plaintext) || err != nil || revoked != nil {
		return nil, ErrSatelliteAuth
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE satellites SET credential_last_used_at=NOW() WHERE id=$1 AND credential_hash=$2 AND credential_revoked_at IS NULL`, satelliteID, encoded); err != nil {
		return nil, err
	}
	return &models.SatelliteIdentity{SatelliteID: satelliteID, TenantID: tenantID}, nil
}

func (r *SatelliteRepository) QueueCommand(ctx context.Context, cmd *models.SatelliteCommand) error {
	cmd.SetDefaults()
	_, _, err := r.queue(ctx, uuid.Nil, cmd, false)
	return err
}
func (r *SatelliteRepository) QueueCommandForTenant(ctx context.Context, tenantID uuid.UUID, cmd *models.SatelliteCommand) error {
	cmd.SetDefaults()
	_, _, err := r.queue(ctx, tenantID, cmd, true)
	return err
}
func (r *SatelliteRepository) QueueCommandIdempotent(ctx context.Context, tenantID uuid.UUID, cmd *models.SatelliteCommand) (*models.SatelliteCommand, bool, error) {
	cmd.SetDefaults()
	return r.queue(ctx, tenantID, cmd, true)
}
func (r *SatelliteRepository) queue(ctx context.Context, tenantID uuid.UUID, cmd *models.SatelliteCommand, scoped bool) (*models.SatelliteCommand, bool, error) {
	where := `s.id=$2`
	args := []any{cmd.ID, cmd.SatelliteID, cmd.Type, cmd.Payload, cmd.Status, cmd.Priority, cmd.CreatedAt, cmd.IdempotencyKey, cmd.MaxAttempts, cmd.AvailableAt}
	if scoped {
		where += ` AND s.tenant_id=$11`
		args = append(args, tenantID)
	}
	query := `INSERT INTO satellite_commands (id,tenant_id,satellite_id,type,payload,status,priority,created_at,idempotency_key,max_attempts,available_at)
		SELECT $1,s.tenant_id,$2,$3,$4,$5,$6,$7,$8,$9,$10 FROM satellites s WHERE ` + where + `
		ON CONFLICT (tenant_id,satellite_id,idempotency_key) DO NOTHING RETURNING ` + commandColumns
	created, err := scanCommand(r.db.QueryRowContext(ctx, query, args...))
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if scoped {
		existing, getErr := r.getCommandByKey(ctx, tenantID, cmd.SatelliteID, cmd.IdempotencyKey)
		if getErr == nil {
			if !sameCommandRequest(existing, cmd) {
				return nil, false, ErrIdempotencyConflict
			}
			return existing, false, nil
		}
		if !errors.Is(getErr, core.ErrNotFound) {
			return nil, false, getErr
		}
	} else {
		existing, getErr := r.getCommandBySatelliteKey(ctx, cmd.SatelliteID, cmd.IdempotencyKey)
		if getErr == nil {
			if !sameCommandRequest(existing, cmd) {
				return nil, false, ErrIdempotencyConflict
			}
			return existing, false, nil
		}
		if !errors.Is(getErr, core.ErrNotFound) {
			return nil, false, getErr
		}
	}
	return nil, false, ErrSatelliteNotFound
}

const commandColumns = `id,tenant_id,satellite_id,idempotency_key,type,payload::text,status,priority,COALESCE(result,''),attempts,execution_generation,max_attempts,available_at,COALESCE(lease_owner,''),lease_token,lease_until,acknowledged_at,COALESCE(last_error,''),terminal_at,created_at,sent_at,completed_at`

func scanCommand(row rowScanner) (*models.SatelliteCommand, error) {
	c := &models.SatelliteCommand{}
	err := row.Scan(&c.ID, &c.TenantID, &c.SatelliteID, &c.IdempotencyKey, &c.Type, &c.Payload, &c.Status, &c.Priority, &c.Result,
		&c.Attempts, &c.ExecutionGeneration, &c.MaxAttempts, &c.AvailableAt, &c.LeaseOwner, &c.LeaseToken, &c.LeaseUntil, &c.AcknowledgedAt, &c.LastError,
		&c.TerminalAt, &c.CreatedAt, &c.SentAt, &c.CompletedAt)
	return c, err
}
func (r *SatelliteRepository) getCommandByKey(ctx context.Context, tenantID, satelliteID uuid.UUID, key string) (*models.SatelliteCommand, error) {
	c, err := scanCommand(r.db.QueryRowContext(ctx, `SELECT `+commandColumns+` FROM satellite_commands WHERE tenant_id=$1 AND satellite_id=$2 AND idempotency_key=$3`, tenantID, satelliteID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	return c, err
}

func (r *SatelliteRepository) getCommandBySatelliteKey(ctx context.Context, satelliteID uuid.UUID, key string) (*models.SatelliteCommand, error) {
	c, err := scanCommand(r.db.QueryRowContext(ctx, `SELECT `+commandColumns+` FROM satellite_commands WHERE satellite_id=$1 AND idempotency_key=$2`, satelliteID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	return c, err
}

func sameCommandRequest(existing, candidate *models.SatelliteCommand) bool {
	if existing.Type != candidate.Type {
		return false
	}
	var existingPayload any
	var candidatePayload any
	if json.Unmarshal([]byte(existing.Payload), &existingPayload) == nil && json.Unmarshal([]byte(candidate.Payload), &candidatePayload) == nil {
		return reflect.DeepEqual(existingPayload, candidatePayload)
	}
	return existing.Payload == candidate.Payload
}

func (r *SatelliteRepository) ClaimCommands(ctx context.Context, satelliteID uuid.UUID, leaseOwner string, limit int, leaseDuration time.Duration) ([]*models.SatelliteCommand, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE satellite_commands SET status='dead_letter',terminal_at=NOW(),last_error=COALESCE(NULLIF(last_error,''),'lease expired after maximum attempts'),lease_owner=NULL,lease_until=NULL
		WHERE satellite_id=$1 AND status IN ('leased','running') AND lease_until<NOW() AND attempts>=max_attempts`, satelliteID)
	if err != nil {
		return nil, err
	}
	until := time.Now().UTC().Add(leaseDuration)
	rows, err := tx.QueryContext(ctx, `WITH candidates AS (
		SELECT id FROM satellite_commands WHERE satellite_id=$1 AND attempts<max_attempts AND
		((status='queued' AND available_at<=NOW()) OR (status IN ('leased','running') AND lease_until<NOW()))
		ORDER BY priority DESC,created_at ASC FOR UPDATE SKIP LOCKED LIMIT $2
	) UPDATE satellite_commands c SET status='leased',attempts=c.attempts+1,lease_owner=$3,lease_token=uuid_generate_v4(),lease_until=$4,sent_at=COALESCE(c.sent_at,NOW()),acknowledged_at=NULL
	FROM candidates WHERE c.id=candidates.id RETURNING `+prefixColumns("c", commandColumns), satelliteID, limit, leaseOwner, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var commands []*models.SatelliteCommand
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		commands = append(commands, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return commands, nil
}

func prefixColumns(prefix, columns string) string {
	// commandColumns contains expressions as well as names; keep this explicit for RETURNING aliases.
	return `c.id,c.tenant_id,c.satellite_id,c.idempotency_key,c.type,c.payload::text,c.status,c.priority,COALESCE(c.result,''),c.attempts,c.execution_generation,c.max_attempts,c.available_at,COALESCE(c.lease_owner,''),c.lease_token,c.lease_until,c.acknowledged_at,COALESCE(c.last_error,''),c.terminal_at,c.created_at,c.sent_at,c.completed_at`
}

func (r *SatelliteRepository) AcknowledgeCommand(ctx context.Context, satelliteID, commandID, leaseToken uuid.UUID, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	res, err := r.db.ExecContext(ctx, `UPDATE satellite_commands SET status='running',acknowledged_at=COALESCE(acknowledged_at,NOW()),lease_until=$1 WHERE satellite_id=$2 AND id=$3 AND lease_token=$4 AND status IN ('leased','running') AND lease_until>=NOW()`, time.Now().UTC().Add(leaseDuration), satelliteID, commandID, leaseToken)
	return requireLeaseAffected(res, err)
}
func (r *SatelliteRepository) RenewCommandLease(ctx context.Context, satelliteID, commandID, leaseToken uuid.UUID, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	res, err := r.db.ExecContext(ctx, `UPDATE satellite_commands SET lease_until=$1 WHERE satellite_id=$2 AND id=$3 AND lease_token=$4 AND status='running' AND lease_until>=NOW()`, time.Now().UTC().Add(leaseDuration), satelliteID, commandID, leaseToken)
	return requireLeaseAffected(res, err)
}
func requireLeaseAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidLease
	}
	return nil
}

func (r *SatelliteRepository) CompleteCommand(ctx context.Context, satelliteID, commandID, leaseToken uuid.UUID, status models.CommandStatus, result, lastError string, retryable bool, backoff time.Duration) (*models.SatelliteCommand, error) {
	if status != models.CommandSucceeded && status != models.CommandFailed {
		return nil, ErrInvalidTransition
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := scanCommand(tx.QueryRowContext(ctx, `SELECT `+commandColumns+` FROM satellite_commands WHERE satellite_id=$1 AND id=$2 FOR UPDATE`, satelliteID, commandID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if current.LeaseToken == nil || *current.LeaseToken != leaseToken {
		return nil, ErrInvalidLease
	}
	if current.Status == models.CommandSucceeded || current.Status == models.CommandFailed || current.Status == models.CommandDeadLetter {
		sameStatus := current.Status == status || (current.Status == models.CommandDeadLetter && status == models.CommandFailed)
		if !sameStatus || current.Result != result || current.LastError != lastError {
			return nil, ErrInvalidTransition
		}
		return current, nil
	}
	if current.Status != models.CommandLeased && current.Status != models.CommandRunning {
		return nil, ErrInvalidTransition
	}
	if current.LeaseUntil == nil || current.LeaseUntil.Before(time.Now().UTC()) {
		return nil, ErrInvalidLease
	}
	next := status
	var available time.Time
	terminal := true
	if status == models.CommandFailed && retryable {
		if current.Attempts >= current.MaxAttempts {
			next = models.CommandDeadLetter
		} else {
			next = models.CommandQueued
			if backoff <= 0 {
				backoff = 5 * time.Second
			}
			shift := current.Attempts - 1
			if shift > 8 {
				shift = 8
			}
			backoff *= time.Duration(1 << shift)
			if backoff > 15*time.Minute {
				backoff = 15 * time.Minute
			}
			available = time.Now().UTC().Add(backoff)
			terminal = false
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE satellite_commands SET status=$1,result=$2,last_error=$3,available_at=CASE WHEN $4::timestamptz IS NULL THEN available_at ELSE $4 END,terminal_at=CASE WHEN $5 THEN NOW() ELSE NULL END,completed_at=CASE WHEN $5 THEN NOW() ELSE NULL END,lease_owner=CASE WHEN $5 THEN lease_owner ELSE NULL END,lease_until=NULL WHERE id=$6`, next, result, lastError, nullableTime(available), terminal, commandID)
	if err != nil {
		return nil, err
	}
	updated, err := scanCommand(tx.QueryRowContext(ctx, `SELECT `+commandColumns+` FROM satellite_commands WHERE id=$1`, commandID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func (r *SatelliteRepository) CancelCommandForTenant(ctx context.Context, tenantID, satelliteID, commandID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `UPDATE satellite_commands SET status='cancelled',terminal_at=NOW(),completed_at=NOW(),lease_until=NULL WHERE tenant_id=$1 AND satellite_id=$2 AND id=$3 AND status IN ('queued','leased')`, tenantID, satelliteID, commandID)
	return requireLeaseAffected(res, err)
}
func (r *SatelliteRepository) RetryCommandForTenant(ctx context.Context, tenantID, satelliteID, commandID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `UPDATE satellite_commands SET status='queued',attempts=0,execution_generation=execution_generation+1,available_at=NOW(),lease_owner=NULL,lease_token=NULL,lease_until=NULL,acknowledged_at=NULL,last_error='',result='',terminal_at=NULL,completed_at=NULL WHERE tenant_id=$1 AND satellite_id=$2 AND id=$3 AND status IN ('failed','dead_letter','cancelled')`, tenantID, satelliteID, commandID)
	return requireLeaseAffected(res, err)
}

// Compatibility reads are non-destructive and do not back the gateway routes.
func (r *SatelliteRepository) GetPendingCommands(ctx context.Context, satelliteID uuid.UUID) ([]*models.SatelliteCommand, error) {
	return r.listCommands(ctx, `SELECT `+commandColumns+` FROM satellite_commands WHERE satellite_id=$1 AND status='queued' ORDER BY priority DESC,created_at`, satelliteID)
}
func (r *SatelliteRepository) GetPendingCommandsForTenant(ctx context.Context, tenantID, satelliteID uuid.UUID) ([]*models.SatelliteCommand, error) {
	return r.listCommands(ctx, `SELECT `+commandColumns+` FROM satellite_commands WHERE tenant_id=$1 AND satellite_id=$2 AND status='queued' ORDER BY priority DESC,created_at`, tenantID, satelliteID)
}
func (r *SatelliteRepository) listCommands(ctx context.Context, query string, args ...any) ([]*models.SatelliteCommand, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.SatelliteCommand
	for rows.Next() {
		c, e := scanCommand(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *SatelliteRepository) UpdateCommand(ctx context.Context, cmd *models.SatelliteCommand) error {
	_, err := r.db.ExecContext(ctx, `UPDATE satellite_commands SET status=$1,result=$2,sent_at=$3,completed_at=$4 WHERE id=$5`, cmd.Status, cmd.Result, cmd.SentAt, cmd.CompletedAt, cmd.ID)
	return err
}
func (r *SatelliteRepository) UpdateCommandForSatellite(ctx context.Context, satelliteID uuid.UUID, cmd *models.SatelliteCommand) error {
	res, err := r.db.ExecContext(ctx, `UPDATE satellite_commands SET status=$1,result=$2,sent_at=$3,completed_at=$4 WHERE id=$5 AND satellite_id=$6`, cmd.Status, cmd.Result, cmd.SentAt, cmd.CompletedAt, cmd.ID, satelliteID)
	return requireLeaseAffected(res, err)
}
