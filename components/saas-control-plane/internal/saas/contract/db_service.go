package contract

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/pkg/models"
	"go.uber.org/zap"
)

var errNotFound = errors.New("record not found")

type postgresService struct {
	db     *sql.DB
	logger *zap.Logger
}

type rowScanner interface {
	Scan(...any) error
}

type tenantRow struct {
	row rowScanner
	tx  *sql.Tx
}

func (r tenantRow) Scan(dest ...any) error {
	if err := r.row.Scan(dest...); err != nil {
		_ = r.tx.Rollback()
		return err
	}
	return r.tx.Commit()
}

type tenantRows struct {
	rows *sql.Rows
	tx   *sql.Tx
}

func (r *tenantRows) Next() bool {
	return r.rows.Next()
}

func (r *tenantRows) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

func (r *tenantRows) Err() error {
	return r.rows.Err()
}
func (r *tenantRows) Close() error {
	rowsErr := r.rows.Err()
	closeErr := r.rows.Close()
	if rowsErr != nil || closeErr != nil {
		_ = r.tx.Rollback()
		if rowsErr != nil {
			return rowsErr
		}
		return closeErr
	}
	return r.tx.Commit()
}

// FORCE RLS requires this transaction-local setting for every operation; using
// a local setting prevents tenant state from leaking through the connection pool.
func (s *postgresService) beginTenantTx(ctx context.Context, tenantID uuid.UUID) (*sql.Tx, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("tenant isolation violation: tenantID cannot be nil")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	var setValue string
	if err := tx.QueryRowContext(ctx,
		"SELECT set_config('app.current_tenant_id', $1, true)",
		tenantID.String()).Scan(&setValue); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("setting transaction-local tenant context: %w", err)
	}
	return tx, nil
}

func (s *postgresService) queryRowContext(ctx context.Context, tenantID uuid.UUID, query string, args ...any) rowScanner {
	tx, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		return tenantError{err: err}
	}
	return tenantRow{row: tx.QueryRowContext(ctx, query, args...), tx: tx}
}

func (s *postgresService) queryContext(ctx context.Context, tenantID uuid.UUID, query string, args ...any) (*tenantRows, error) {
	tx, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return &tenantRows{rows: rows, tx: tx}, nil
}

type tenantError struct {
	err error
}

func (e tenantError) Scan(...any) error {
	return e.err
}

// NewPostgresService creates a service that uses PostgreSQL and never falls back
// to the in-memory implementation.
func NewPostgresService(db *sql.DB, logger *zap.Logger) (Service, error) {
	if db == nil {
		return nil, errors.New("contract service requires a database")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &postgresService{db: db, logger: logger}, nil
}

var _ Service = (*postgresService)(nil)

const contractColumns = `id, tenant_id, contract_number, title, type, status, stage,
	counterparty_name, COALESCE(counterparty_contact,''), COALESCE(counterparty_email,''),
	COALESCE(entity_id,''), COALESCE(business_unit,''), COALESCE(department,''),
	COALESCE(owner_user_id,''), COALESCE(owner_email,''), effective_date, expiration_date,
	execution_date, COALESCE(renewal_type,''), COALESCE(renewal_notice_days,0),
	COALESCE(notice_period_days,0), COALESCE(total_value,0), COALESCE(currency,''),
	COALESCE(payment_terms,''), COALESCE(governing_law,''), COALESCE(jurisdiction,''),
	COALESCE(risk_score,0), COALESCE(compliance_status,''), COALESCE(current_version,1),
	COALESCE(content_body,''), COALESCE(folder_path,''), parent_contract_id,
	master_agreement_id, COALESCE(tags,'[]'::jsonb), COALESCE(metadata,'{}'::jsonb),
	created_at, updated_at`

func scanContract(row interface{ Scan(...any) error }) (*models.Contract, error) {
	var c models.Contract
	var effective, expiration, execution sql.NullTime
	var parentID, masterID sql.NullString
	var tags, metadata []byte
	err := row.Scan(
		&c.ID, &c.TenantID, &c.ContractNumber, &c.Title, &c.Type, &c.Status, &c.Stage,
		&c.CounterpartyName, &c.CounterpartyContact, &c.CounterpartyEmail, &c.EntityID,
		&c.BusinessUnit, &c.Department, &c.OwnerUserID, &c.OwnerEmail, &effective,
		&expiration, &execution, &c.RenewalType, &c.RenewalNoticeDays, &c.NoticePeriodDays,
		&c.TotalValue, &c.Currency, &c.PaymentTerms, &c.GoverningLaw, &c.Jurisdiction,
		&c.RiskScore, &c.ComplianceStatus, &c.CurrentVersion, &c.ContentBody, &c.FolderPath,
		&parentID, &masterID, &tags, &metadata, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if effective.Valid {
		c.EffectiveDate = &effective.Time
	}
	if expiration.Valid {
		c.ExpirationDate = &expiration.Time
	}
	if execution.Valid {
		c.ExecutionDate = &execution.Time
	}
	if parentID.Valid {
		id, parseErr := uuid.Parse(parentID.String)
		if parseErr != nil {
			return nil, parseErr
		}
		c.ParentContractID = &id
	}
	if masterID.Valid {
		id, parseErr := uuid.Parse(masterID.String)
		if parseErr != nil {
			return nil, parseErr
		}
		c.MasterAgreementID = &id
	}
	_ = json.Unmarshal(tags, &c.Tags)
	_ = json.Unmarshal(metadata, &c.Metadata)
	return &c, nil
}

func (s *postgresService) ListContracts(ctx context.Context, tenantID uuid.UUID, filter Filter) ([]*models.Contract, int, error) {
	where := []string{"tenant_id = $1"}
	args := []any{tenantID}
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	if filter.Type != "" {
		add("type = $%d", filter.Type)
	}
	if filter.Status != "" {
		add("status = $%d", filter.Status)
	}
	if filter.Stage != "" {
		add("stage = $%d", filter.Stage)
	}
	if filter.Counterparty != "" {
		add("counterparty_name ILIKE $%d ESCAPE E'\\\\'", "%"+escapeLike(filter.Counterparty)+"%")
	}
	if filter.Search != "" {
		needle := "%" + escapeLike(filter.Search) + "%"
		args = append(args, needle)
		n := len(args)
		where = append(where, fmt.Sprintf("(title ILIKE $%d ESCAPE E'\\\\' OR contract_number ILIKE $%d ESCAPE E'\\\\' OR counterparty_name ILIKE $%d ESCAPE E'\\\\')", n, n, n))
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.queryRowContext(ctx, tenantID, `SELECT count(*) FROM contracts WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append([]any(nil), args...)
	query := `SELECT ` + contractColumns + ` FROM contracts WHERE ` + clause + ` ORDER BY created_at DESC`
	if filter.Limit > 0 {
		queryArgs = append(queryArgs, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(queryArgs))
	}
	if filter.Offset > 0 {
		queryArgs = append(queryArgs, filter.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(queryArgs))
	}
	rows, err := s.queryContext(ctx, tenantID, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]*models.Contract, 0)
	for rows.Next() {
		c, scanErr := scanContract(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, c)
	}
	return result, total, rows.Err()
}

func (s *postgresService) GetContract(ctx context.Context, tenantID, contractID uuid.UUID) (*models.Contract, error) {
	c, err := scanContract(s.queryRowContext(ctx, tenantID, `SELECT `+contractColumns+` FROM contracts WHERE tenant_id=$1 AND id=$2`, tenantID, contractID))
	return c, notFound(err)
}

func (s *postgresService) CreateContract(ctx context.Context, tenantID uuid.UUID, c *models.Contract) (*models.Contract, error) {
	if c == nil {
		return nil, errors.New("contract is required")
	}
	now := time.Now().UTC()
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.TenantID = tenantID
	if c.ContractNumber == "" {
		c.ContractNumber = fmt.Sprintf("CNT-%d-%s", now.Year(), strings.ToUpper(strings.ReplaceAll(c.ID.String()[:8], "-", "")))
	}
	if c.Status == "" {
		c.Status = models.StatusDraft
	}
	if c.Stage == "" {
		c.Stage = models.StageAuthoring
	}
	c.CurrentVersion, c.CreatedAt, c.UpdatedAt = 1, now, now
	tx, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := `INSERT INTO contracts (id,tenant_id,contract_number,title,type,status,stage,counterparty_name,
	counterparty_contact,counterparty_email,entity_id,business_unit,department,owner_user_id,owner_email,
	effective_date,expiration_date,execution_date,renewal_type,renewal_notice_days,notice_period_days,total_value,
	currency,payment_terms,governing_law,jurisdiction,risk_score,compliance_status,current_version,content_body,
	folder_path,parent_contract_id,master_agreement_id,tags,metadata,created_at,updated_at)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37)
	RETURNING ` + contractColumns
	created, err := scanContract(tx.QueryRowContext(ctx, query,
		c.ID, tenantID, c.ContractNumber, c.Title, c.Type, c.Status, c.Stage, c.CounterpartyName,
		nullableString(c.CounterpartyContact), nullableString(c.CounterpartyEmail), nullableString(c.EntityID),
		nullableString(c.BusinessUnit), nullableString(c.Department), nullableString(c.OwnerUserID),
		nullableString(c.OwnerEmail), c.EffectiveDate, c.ExpirationDate, c.ExecutionDate,
		nullableString(c.RenewalType), c.RenewalNoticeDays, c.NoticePeriodDays, c.TotalValue,
		nullableString(c.Currency), nullableString(c.PaymentTerms), nullableString(c.GoverningLaw),
		nullableString(c.Jurisdiction), c.RiskScore, nullableString(c.ComplianceStatus), c.CurrentVersion,
		c.ContentBody, nullableString(c.FolderPath), c.ParentContractID, c.MasterAgreementID,
		jsonValue(c.Tags, []string{}), jsonValue(c.Metadata, map[string]interface{}{}), now, now))
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_versions
		(id,contract_id,version_number,title,content_body,changelog_summary,author_email,document_hash,created_at)
		VALUES ($1,$2,1,$3,$4,$5,$6,$7,$8)`, uuid.New(), c.ID, c.Title, c.ContentBody,
		"Initial contract authoring baseline", c.OwnerEmail, hashContent(c.ContentBody), now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *postgresService) UpdateContract(ctx context.Context, tenantID uuid.UUID, c *models.Contract) (*models.Contract, error) {
	if c == nil {
		return nil, errors.New("contract is required")
	}
	now := time.Now().UTC()
	query := `UPDATE contracts SET contract_number=$3,title=$4,type=$5,status=$6,stage=$7,counterparty_name=$8,
	counterparty_contact=$9,counterparty_email=$10,entity_id=$11,business_unit=$12,department=$13,
	owner_user_id=$14,owner_email=$15,effective_date=$16,expiration_date=$17,execution_date=$18,
	renewal_type=$19,renewal_notice_days=$20,notice_period_days=$21,total_value=$22,currency=$23,
	payment_terms=$24,governing_law=$25,jurisdiction=$26,risk_score=$27,compliance_status=$28,
	current_version=$29,content_body=$30,folder_path=$31,parent_contract_id=$32,master_agreement_id=$33,
	tags=$34,metadata=$35,updated_at=$36 WHERE tenant_id=$1 AND id=$2 RETURNING ` + contractColumns
	updated, err := scanContract(s.queryRowContext(ctx, tenantID, query, tenantID, c.ID, c.ContractNumber, c.Title,
		c.Type, c.Status, c.Stage, c.CounterpartyName, nullableString(c.CounterpartyContact),
		nullableString(c.CounterpartyEmail), nullableString(c.EntityID), nullableString(c.BusinessUnit),
		nullableString(c.Department), nullableString(c.OwnerUserID), nullableString(c.OwnerEmail),
		c.EffectiveDate, c.ExpirationDate, c.ExecutionDate, nullableString(c.RenewalType),
		c.RenewalNoticeDays, c.NoticePeriodDays, c.TotalValue, nullableString(c.Currency),
		nullableString(c.PaymentTerms), nullableString(c.GoverningLaw), nullableString(c.Jurisdiction),
		c.RiskScore, nullableString(c.ComplianceStatus), c.CurrentVersion, c.ContentBody,
		nullableString(c.FolderPath), c.ParentContractID, c.MasterAgreementID,
		jsonValue(c.Tags, []string{}), jsonValue(c.Metadata, map[string]interface{}{}), now))
	return updated, notFound(err)
}

func (s *postgresService) TransitionContract(ctx context.Context, tenantID, contractID uuid.UUID, targetStatus models.ContractStatus, targetStage models.ContractStage, comment string) (*models.Contract, error) {
	if targetStage == "" {
		c, err := scanContract(s.queryRowContext(ctx, tenantID, `UPDATE contracts SET status=$3,updated_at=$4 WHERE tenant_id=$1 AND id=$2 RETURNING `+contractColumns,
			tenantID, contractID, targetStatus, time.Now().UTC()))
		return c, notFound(err)
	}
	c, err := scanContract(s.queryRowContext(ctx, tenantID, `UPDATE contracts SET status=$3,stage=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2 RETURNING `+contractColumns,
		tenantID, contractID, targetStatus, targetStage, time.Now().UTC()))
	if err == nil {
		s.logger.Info("contract transitioned", zap.String("contract_id", contractID.String()), zap.String("new_status", string(targetStatus)), zap.String("comment", comment))
	}
	return c, notFound(err)
}

func (s *postgresService) ListVersions(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractVersion, error) {
	rows, err := s.queryContext(ctx, tenantID, `SELECT v.id,v.contract_id,v.version_number,v.title,v.content_body,
		COALESCE(v.changelog_summary,''),COALESCE(v.author_user_id,''),COALESCE(v.author_email,''),
		COALESCE(v.document_hash,''),v.created_at FROM contract_versions v
		JOIN contracts c ON c.id=v.contract_id WHERE c.tenant_id=$1 AND c.id=$2 ORDER BY v.version_number`, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*models.ContractVersion, 0)
	for rows.Next() {
		v := new(models.ContractVersion)
		if err := rows.Scan(&v.ID, &v.ContractID, &v.VersionNumber, &v.Title, &v.ContentBody,
			&v.ChangelogSummary, &v.AuthorUserID, &v.AuthorEmail, &v.DocumentHash, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *postgresService) CreateVersion(ctx context.Context, tenantID, contractID uuid.UUID, title, content, changelog, authorEmail string) (*models.ContractVersion, error) {
	tx, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var current int
	err = tx.QueryRowContext(ctx, `SELECT current_version FROM contracts WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, contractID).Scan(&current)
	if err != nil {
		return nil, notFound(err)
	}
	now, next := time.Now().UTC(), current+1
	v := &models.ContractVersion{ID: uuid.New(), ContractID: contractID, VersionNumber: next, Title: title,
		ContentBody: content, ChangelogSummary: changelog, AuthorEmail: authorEmail, DocumentHash: hashContent(content), CreatedAt: now}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_versions (id,contract_id,version_number,title,content_body,changelog_summary,author_email,document_hash,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, v.ID, contractID, next, title, content, changelog, authorEmail, v.DocumentHash, now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contracts SET current_version=$3,content_body=$4,updated_at=$5 WHERE tenant_id=$1 AND id=$2`, tenantID, contractID, next, content, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *postgresService) ListRequests(ctx context.Context, tenantID uuid.UUID) ([]*models.ContractRequest, error) {
	rows, err := s.queryContext(ctx, tenantID, `SELECT id,tenant_id,request_number,title,contract_type,COALESCE(request_origin,''),
		counterparty_name,COALESCE(counterparty_email,''),COALESCE(estimated_value,0),COALESCE(currency,''),
		COALESCE(urgency,''),COALESCE(business_justification,''),COALESCE(department,''),requester_email,
		COALESCE(assigned_counsel_email,''),COALESCE(status,''),converted_contract_id,sla_deadline,
		COALESCE(form_data,'{}'::jsonb),created_at,updated_at FROM contract_requests WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*models.ContractRequest, 0)
	for rows.Next() {
		r, scanErr := scanRequest(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func scanRequest(row interface{ Scan(...any) error }) (*models.ContractRequest, error) {
	r := new(models.ContractRequest)
	var converted sql.NullString
	var deadline sql.NullTime
	var data []byte
	err := row.Scan(&r.ID, &r.TenantID, &r.RequestNumber, &r.Title, &r.ContractType, &r.RequestOrigin,
		&r.CounterpartyName, &r.CounterpartyEmail, &r.EstimatedValue, &r.Currency, &r.Urgency,
		&r.BusinessJustification, &r.Department, &r.RequesterEmail, &r.AssignedCounselEmail,
		&r.Status, &converted, &deadline, &data, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if converted.Valid {
		id, parseErr := uuid.Parse(converted.String)
		if parseErr != nil {
			return nil, parseErr
		}
		r.ConvertedContractID = &id
	}
	if deadline.Valid {
		r.SLADeadline = &deadline.Time
	}
	_ = json.Unmarshal(data, &r.FormData)
	return r, nil
}

func (s *postgresService) SubmitRequest(ctx context.Context, tenantID uuid.UUID, req *models.ContractRequest) (*models.ContractRequest, error) {
	if req == nil {
		return nil, errors.New("contract request is required")
	}
	if req.ID == uuid.Nil {
		req.ID = uuid.New()
	}
	now := time.Now().UTC()
	req.TenantID, req.Status, req.CreatedAt, req.UpdatedAt = tenantID, "submitted", now, now
	if req.RequestNumber == "" {
		req.RequestNumber = fmt.Sprintf("REQ-%d-%s", now.Year(), strings.ToUpper(req.ID.String()[:8]))
	}
	return scanRequest(s.queryRowContext(ctx, tenantID, `INSERT INTO contract_requests
		(id,tenant_id,request_number,title,contract_type,request_origin,counterparty_name,counterparty_email,
		estimated_value,currency,urgency,business_justification,department,requester_email,assigned_counsel_email,
		status,converted_contract_id,sla_deadline,form_data,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		RETURNING id,tenant_id,request_number,title,contract_type,COALESCE(request_origin,''),
		counterparty_name,COALESCE(counterparty_email,''),COALESCE(estimated_value,0),COALESCE(currency,''),
		COALESCE(urgency,''),COALESCE(business_justification,''),COALESCE(department,''),requester_email,
		COALESCE(assigned_counsel_email,''),COALESCE(status,''),converted_contract_id,sla_deadline,
		COALESCE(form_data,'{}'::jsonb),created_at,updated_at`,
		req.ID, tenantID, req.RequestNumber, req.Title, req.ContractType, nullableString(req.RequestOrigin),
		req.CounterpartyName, nullableString(req.CounterpartyEmail), req.EstimatedValue, nullableString(req.Currency),
		nullableString(req.Urgency), nullableString(req.BusinessJustification), nullableString(req.Department),
		req.RequesterEmail, nullableString(req.AssignedCounselEmail), req.Status, req.ConvertedContractID,
		req.SLADeadline, jsonValue(req.FormData, map[string]interface{}{}), now, now))
}

func (s *postgresService) TriageRequest(ctx context.Context, tenantID, reqID uuid.UUID, assignedCounsel, status string) (*models.ContractRequest, error) {
	return scanRequest(s.queryRowContext(ctx, tenantID, `UPDATE contract_requests SET
		assigned_counsel_email=CASE WHEN $3='' THEN assigned_counsel_email ELSE $3 END,
		status=CASE WHEN $4='' THEN status ELSE $4 END,updated_at=$5
		WHERE tenant_id=$1 AND id=$2
		RETURNING id,tenant_id,request_number,title,contract_type,COALESCE(request_origin,''),
		counterparty_name,COALESCE(counterparty_email,''),COALESCE(estimated_value,0),COALESCE(currency,''),
		COALESCE(urgency,''),COALESCE(business_justification,''),COALESCE(department,''),requester_email,
		COALESCE(assigned_counsel_email,''),COALESCE(status,''),converted_contract_id,sla_deadline,
		COALESCE(form_data,'{}'::jsonb),created_at,updated_at`, tenantID, reqID, assignedCounsel, status, time.Now().UTC()))
}

func (s *postgresService) ConvertRequestToContract(ctx context.Context, tenantID, reqID uuid.UUID) (*models.Contract, error) {
	tx, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	req, err := scanRequest(tx.QueryRowContext(ctx, `SELECT id,tenant_id,request_number,title,contract_type,
		COALESCE(request_origin,''),counterparty_name,COALESCE(counterparty_email,''),COALESCE(estimated_value,0),
		COALESCE(currency,''),COALESCE(urgency,''),COALESCE(business_justification,''),COALESCE(department,''),
		requester_email,COALESCE(assigned_counsel_email,''),COALESCE(status,''),converted_contract_id,sla_deadline,
		COALESCE(form_data,'{}'::jsonb),created_at,updated_at FROM contract_requests WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, reqID))
	if err != nil {
		return nil, notFound(err)
	}
	if req.Status == "converted" {
		return nil, errors.New("contract request has already been converted")
	}
	now := time.Now().UTC()
	c := &models.Contract{ID: uuid.New(), TenantID: tenantID,
		ContractNumber: fmt.Sprintf("CNT-%d-%s", now.Year(), strings.ToUpper(uuid.New().String()[:8])),
		Title: req.Title, Type: models.ContractType(req.ContractType), Status: models.StatusDraft,
		Stage: models.StageAuthoring, CounterpartyName: req.CounterpartyName, TotalValue: req.EstimatedValue,
		Currency: req.Currency, Department: req.Department, OwnerEmail: req.RequesterEmail,
		CurrentVersion: 1, ContentBody: fmt.Sprintf("Contract generated from Request %s: %s\n\nJustification: %s", req.RequestNumber, req.Title, req.BusinessJustification),
		CreatedAt: now, UpdatedAt: now}
	created, err := insertContract(ctx, tx, c)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_versions (id,contract_id,version_number,title,content_body,changelog_summary,author_email,document_hash,created_at)
		VALUES ($1,$2,1,$3,$4,$5,$6,$7,$8)`, uuid.New(), c.ID, c.Title, c.ContentBody,
		"Initial contract authoring baseline", c.OwnerEmail, hashContent(c.ContentBody), now); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE contract_requests SET status='converted',converted_contract_id=$3,updated_at=$4 WHERE tenant_id=$1 AND id=$2`, tenantID, reqID, c.ID, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func insertContract(ctx context.Context, tx *sql.Tx, c *models.Contract) (*models.Contract, error) {
	query := `INSERT INTO contracts (id,tenant_id,contract_number,title,type,status,stage,counterparty_name,
		counterparty_contact,counterparty_email,entity_id,business_unit,department,owner_user_id,owner_email,
		effective_date,expiration_date,execution_date,renewal_type,renewal_notice_days,notice_period_days,total_value,
		currency,payment_terms,governing_law,jurisdiction,risk_score,compliance_status,current_version,content_body,
		folder_path,parent_contract_id,master_agreement_id,tags,metadata,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37)
		RETURNING ` + contractColumns
	return scanContract(tx.QueryRowContext(ctx, query, c.ID, c.TenantID, c.ContractNumber, c.Title, c.Type,
		c.Status, c.Stage, c.CounterpartyName, nullableString(c.CounterpartyContact), nullableString(c.CounterpartyEmail),
		nullableString(c.EntityID), nullableString(c.BusinessUnit), nullableString(c.Department), nullableString(c.OwnerUserID),
		nullableString(c.OwnerEmail), c.EffectiveDate, c.ExpirationDate, c.ExecutionDate, nullableString(c.RenewalType),
		c.RenewalNoticeDays, c.NoticePeriodDays, c.TotalValue, nullableString(c.Currency), nullableString(c.PaymentTerms),
		nullableString(c.GoverningLaw), nullableString(c.Jurisdiction), c.RiskScore, nullableString(c.ComplianceStatus),
		c.CurrentVersion, c.ContentBody, nullableString(c.FolderPath), c.ParentContractID, c.MasterAgreementID,
		jsonValue(c.Tags, []string{}), jsonValue(c.Metadata, map[string]interface{}{}), c.CreatedAt, c.UpdatedAt))
}

func (s *postgresService) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]*models.ContractTemplate, error) {
	rows, err := s.queryContext(ctx, tenantID, `SELECT id,tenant_id,name,category,COALESCE(description,''),content_body,
		COALESCE(version,1),COALESCE(status,''),COALESCE(variables,'[]'::jsonb),
		COALESCE(governing_rules,'{}'::jsonb),COALESCE(created_by,''),created_at,updated_at
		FROM contract_templates WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*models.ContractTemplate, 0)
	for rows.Next() {
		t := new(models.ContractTemplate)
		var variables, rules []byte
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Category, &t.Description, &t.ContentBody,
			&t.Version, &t.Status, &variables, &rules, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(variables, &t.Variables)
		_ = json.Unmarshal(rules, &t.GoverningRules)
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *postgresService) CreateTemplate(ctx context.Context, tenantID uuid.UUID, tpl *models.ContractTemplate) (*models.ContractTemplate, error) {
	if tpl == nil {
		return nil, errors.New("contract template is required")
	}
	if tpl.ID == uuid.Nil {
		tpl.ID = uuid.New()
	}
	now := time.Now().UTC()
	tpl.TenantID, tpl.CreatedAt, tpl.UpdatedAt = tenantID, now, now
	return scanTemplate(s.queryRowContext(ctx, tenantID, `INSERT INTO contract_templates
		(id,tenant_id,name,category,description,content_body,version,status,variables,governing_rules,created_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id,tenant_id,name,category,COALESCE(description,''),content_body,COALESCE(version,1),
		COALESCE(status,''),COALESCE(variables,'[]'::jsonb),COALESCE(governing_rules,'{}'::jsonb),
		COALESCE(created_by,''),created_at,updated_at`, tpl.ID, tenantID, tpl.Name, tpl.Category,
		nullableString(tpl.Description), tpl.ContentBody, tpl.Version, nullableString(tpl.Status),
		jsonValue(tpl.Variables, []string{}), jsonValue(tpl.GoverningRules, map[string]interface{}{}),
		nullableString(tpl.CreatedBy), now, now))
}

func scanTemplate(row interface{ Scan(...any) error }) (*models.ContractTemplate, error) {
	t := new(models.ContractTemplate)
	var variables, rules []byte
	err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Category, &t.Description, &t.ContentBody, &t.Version,
		&t.Status, &variables, &rules, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(variables, &t.Variables)
	_ = json.Unmarshal(rules, &t.GoverningRules)
	return t, nil
}

func (s *postgresService) ListClauses(ctx context.Context, tenantID uuid.UUID, category string) ([]*models.Clause, error) {
	query := `SELECT id,tenant_id,title,category,standard_text,COALESCE(fallback_text,''),COALESCE(risk_level,''),
		COALESCE(is_mandatory,false),COALESCE(playbook_guidance,''),COALESCE(jurisdiction_tags,'[]'::jsonb),
		COALESCE(approval_role_required,''),created_at,updated_at FROM clause_library WHERE tenant_id=$1`
	args := []any{tenantID}
	if category != "" {
		query += ` AND lower(category)=lower($2)`
		args = append(args, category)
	}
	rows, err := s.queryContext(ctx, tenantID, query+` ORDER BY title`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*models.Clause, 0)
	for rows.Next() {
		cl := new(models.Clause)
		var tags []byte
		if err := rows.Scan(&cl.ID, &cl.TenantID, &cl.Title, &cl.Category, &cl.StandardText, &cl.FallbackText,
			&cl.RiskLevel, &cl.IsMandatory, &cl.PlaybookGuidance, &tags, &cl.ApprovalRoleRequired,
			&cl.CreatedAt, &cl.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tags, &cl.JurisdictionTags)
		list = append(list, cl)
	}
	return list, rows.Err()
}

func (s *postgresService) CreateClause(ctx context.Context, tenantID uuid.UUID, cl *models.Clause) (*models.Clause, error) {
	if cl == nil {
		return nil, errors.New("clause is required")
	}
	if cl.ID == uuid.Nil {
		cl.ID = uuid.New()
	}
	now := time.Now().UTC()
	cl.TenantID, cl.CreatedAt, cl.UpdatedAt = tenantID, now, now
	return scanClause(s.queryRowContext(ctx, tenantID, `INSERT INTO clause_library
		(id,tenant_id,title,category,standard_text,fallback_text,risk_level,is_mandatory,playbook_guidance,jurisdiction_tags,approval_role_required,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id,tenant_id,title,category,standard_text,COALESCE(fallback_text,''),COALESCE(risk_level,''),
		COALESCE(is_mandatory,false),COALESCE(playbook_guidance,''),COALESCE(jurisdiction_tags,'[]'::jsonb),
		COALESCE(approval_role_required,''),created_at,updated_at`, cl.ID, tenantID, cl.Title, cl.Category,
		cl.StandardText, nullableString(cl.FallbackText), nullableString(cl.RiskLevel), cl.IsMandatory,
		nullableString(cl.PlaybookGuidance), jsonValue(cl.JurisdictionTags, []string{}),
		nullableString(cl.ApprovalRoleRequired), now, now))
}

func scanClause(row interface{ Scan(...any) error }) (*models.Clause, error) {
	cl := new(models.Clause)
	var tags []byte
	err := row.Scan(&cl.ID, &cl.TenantID, &cl.Title, &cl.Category, &cl.StandardText, &cl.FallbackText,
		&cl.RiskLevel, &cl.IsMandatory, &cl.PlaybookGuidance, &tags, &cl.ApprovalRoleRequired, &cl.CreatedAt, &cl.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(tags, &cl.JurisdictionTags)
	return cl, nil
}

func (s *postgresService) AssembleContractFromTemplate(ctx context.Context, tenantID, templateID uuid.UUID, variables map[string]string, selectedClauseIDs []uuid.UUID) (string, error) {
	var body string
	if err := s.queryRowContext(ctx, tenantID, `SELECT content_body FROM contract_templates WHERE tenant_id=$1 AND id=$2`, tenantID, templateID).Scan(&body); err != nil {
		return "", notFound(err)
	}
	body = applyVariables(body, variables)
	if len(selectedClauseIDs) == 0 {
		return body, nil
	}
	body += "\n\n--- ADDITIONAL PRE-APPROVED CLAUSES ---\n"
	for _, id := range selectedClauseIDs {
		var title, text string
		err := s.queryRowContext(ctx, tenantID, `SELECT title,standard_text FROM clause_library WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&title, &text)
		if err == nil {
			body += fmt.Sprintf("\n\n[%s]\n%s\n", title, text)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return body, nil
}

func (s *postgresService) ListRedlines(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractRedline, error) {
	rows, err := s.queryContext(ctx, tenantID, `SELECT r.id,r.contract_id,r.negotiation_id,COALESCE(r.author_name,''),
		COALESCE(r.author_type,''),COALESCE(r.change_type,''),COALESCE(r.clause_title,''),
		COALESCE(r.original_text,''),COALESCE(r.proposed_text,''),COALESCE(r.status,''),
		COALESCE(r.review_comments,''),r.created_at,r.resolved_at FROM contract_redlines r
		JOIN contracts c ON c.id=r.contract_id WHERE c.tenant_id=$1 AND c.id=$2 ORDER BY r.created_at`, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*models.ContractRedline, 0)
	for rows.Next() {
		r, scanErr := scanRedline(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func scanRedline(row interface{ Scan(...any) error }) (*models.ContractRedline, error) {
	r := new(models.ContractRedline)
	var negotiation sql.NullString
	var resolved sql.NullTime
	err := row.Scan(&r.ID, &r.ContractID, &negotiation, &r.AuthorName, &r.AuthorType, &r.ChangeType,
		&r.ClauseTitle, &r.OriginalText, &r.ProposedText, &r.Status, &r.ReviewComments, &r.CreatedAt, &resolved)
	if err != nil {
		return nil, err
	}
	if negotiation.Valid {
		id, e := uuid.Parse(negotiation.String)
		if e != nil {
			return nil, e
		}
		r.NegotiationID = &id
	}
	if resolved.Valid {
		r.ResolvedAt = &resolved.Time
	}
	return r, nil
}

func (s *postgresService) SubmitRedline(ctx context.Context, tenantID, contractID uuid.UUID, r *models.ContractRedline) (*models.ContractRedline, error) {
	if r == nil {
		return nil, errors.New("redline is required")
	}
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	r.ContractID, r.Status, r.CreatedAt = contractID, "proposed", time.Now().UTC()
	r, err := scanRedline(s.queryRowContext(ctx, tenantID, `INSERT INTO contract_redlines
		(id,contract_id,negotiation_id,author_name,author_type,change_type,clause_title,original_text,proposed_text,status,review_comments,created_at)
		SELECT $1,c.id,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 FROM contracts c WHERE c.tenant_id=$2 AND c.id=$13
		RETURNING id,contract_id,negotiation_id,COALESCE(author_name,''),COALESCE(author_type,''),COALESCE(change_type,''),
		COALESCE(clause_title,''),COALESCE(original_text,''),COALESCE(proposed_text,''),COALESCE(status,''),
		COALESCE(review_comments,''),created_at,resolved_at`,
		r.ID, tenantID, r.NegotiationID, r.AuthorName, nullableString(r.AuthorType), nullableString(r.ChangeType),
		nullableString(r.ClauseTitle), nullableString(r.OriginalText), nullableString(r.ProposedText), r.Status,
		nullableString(r.ReviewComments), r.CreatedAt, contractID))
	return r, notFound(err)
}

func (s *postgresService) ResolveRedline(ctx context.Context, tenantID, redlineID uuid.UUID, accepted bool, reviewComment string) (*models.ContractRedline, error) {
	status := "rejected"
	if accepted {
		status = "accepted"
	}
	return scanRedline(s.queryRowContext(ctx, tenantID, `UPDATE contract_redlines r SET status=$3,review_comments=$4,resolved_at=$5
		FROM contracts c WHERE c.id=r.contract_id AND c.tenant_id=$1 AND r.id=$2
		RETURNING r.id,r.contract_id,r.negotiation_id,COALESCE(r.author_name,''),COALESCE(r.author_type,''),
		COALESCE(r.change_type,''),COALESCE(r.clause_title,''),COALESCE(r.original_text,''),
		COALESCE(r.proposed_text,''),COALESCE(r.status,''),COALESCE(r.review_comments,''),r.created_at,r.resolved_at`,
		tenantID, redlineID, status, reviewComment, time.Now().UTC()))
}

func (s *postgresService) ListSignatures(ctx context.Context, tenantID, contractID uuid.UUID) ([]*models.ContractSignature, error) {
	rows, err := s.queryContext(ctx, tenantID, `SELECT s.id,s.contract_id,COALESCE(s.signer_name,''),COALESCE(s.signer_email,''),
		COALESCE(s.signer_role,''),COALESCE(s.signing_order,1),COALESCE(s.status,''),COALESCE(s.auth_method,''),
		COALESCE(s.ip_address,''),COALESCE(s.user_agent,''),COALESCE(s.digital_signature_hash,''),
		COALESCE(s.certificate_thumbprint,''),s.signed_at,COALESCE(s.evidence_bundle_uri,''),s.created_at
		FROM contract_signatures s JOIN contracts c ON c.id=s.contract_id
		WHERE c.tenant_id=$1 AND c.id=$2 ORDER BY s.signing_order,s.created_at`, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*models.ContractSignature, 0)
	for rows.Next() {
		sig, scanErr := scanSignature(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list = append(list, sig)
	}
	return list, rows.Err()
}

func scanSignature(row interface{ Scan(...any) error }) (*models.ContractSignature, error) {
	sig := new(models.ContractSignature)
	var signed sql.NullTime
	err := row.Scan(&sig.ID,&sig.ContractID,&sig.SignerName,&sig.SignerEmail,&sig.SignerRole,&sig.SigningOrder,
		&sig.Status,&sig.AuthMethod,&sig.IPAddress,&sig.UserAgent,&sig.DigitalSignatureHash,
		&sig.CertificateThumbprint,&signed,&sig.EvidenceBundleURI,&sig.CreatedAt)
	if err != nil {
		return nil, err
	}
	if signed.Valid {
		sig.SignedAt = &signed.Time
	}
	return sig, nil
}

func (s *postgresService) AddSigner(ctx context.Context, tenantID, contractID uuid.UUID, sig *models.ContractSignature) (*models.ContractSignature, error) {
	if sig == nil {
		return nil, errors.New("signature is required")
	}
	if sig.ID == uuid.Nil {
		sig.ID = uuid.New()
	}
	sig.ContractID, sig.Status, sig.CreatedAt = contractID, "pending", time.Now().UTC()
	return scanSignature(s.queryRowContext(ctx, tenantID, `INSERT INTO contract_signatures
		(id,contract_id,signer_name,signer_email,signer_role,signing_order,status,auth_method,ip_address,user_agent,
		digital_signature_hash,certificate_thumbprint,signed_at,evidence_bundle_uri,created_at)
		SELECT $1,c.id,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15 FROM contracts c WHERE c.tenant_id=$2 AND c.id=$16
		RETURNING id,contract_id,COALESCE(signer_name,''),COALESCE(signer_email,''),COALESCE(signer_role,''),
		COALESCE(signing_order,1),COALESCE(status,''),COALESCE(auth_method,''),COALESCE(ip_address,''),
		COALESCE(user_agent,''),COALESCE(digital_signature_hash,''),COALESCE(certificate_thumbprint,''),
		signed_at,COALESCE(evidence_bundle_uri,''),created_at`, sig.ID, tenantID, sig.SignerName, sig.SignerEmail,
		nullableString(sig.SignerRole), sig.SigningOrder, sig.Status, nullableString(sig.AuthMethod),
		nullableString(sig.IPAddress), nullableString(sig.UserAgent), nullableString(sig.DigitalSignatureHash),
		nullableString(sig.CertificateThumbprint), sig.SignedAt, nullableString(sig.EvidenceBundleURI), sig.CreatedAt, contractID))
}

func (s *postgresService) ExecuteSignature(ctx context.Context, tenantID, sigID uuid.UUID, authMethod, ipAddress string) (*models.ContractSignature, error) {
	tx, err := s.beginTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var email, status string
	err = tx.QueryRowContext(ctx, `SELECT s.signer_email,s.status FROM contract_signatures s
		JOIN contracts c ON c.id=s.contract_id WHERE c.tenant_id=$1 AND s.id=$2 FOR UPDATE OF s`, tenantID, sigID).Scan(&email,&status)
	if err != nil {
		return nil, notFound(err)
	}
	if status == "signed" {
		return nil, errors.New("CLM-CRYPTO-ERR-001: target signature has already been executed (duplicate signing prohibited)")
	}
	now := time.Now().UTC()
	hash := hashContent(fmt.Sprintf("%s:%s:%s", email, now.String(), ipAddress))
	sig, err := scanSignature(tx.QueryRowContext(ctx, `UPDATE contract_signatures AS sig SET status='signed',auth_method=$3,
		ip_address=$4,signed_at=$5,digital_signature_hash=$6 WHERE sig.id=$2 AND EXISTS
		(SELECT 1 FROM contracts c WHERE c.id=sig.contract_id AND c.tenant_id=$1)
		RETURNING sig.id,sig.contract_id,COALESCE(sig.signer_name,''),COALESCE(sig.signer_email,''),COALESCE(sig.signer_role,''),
		COALESCE(sig.signing_order,1),COALESCE(sig.status,''),COALESCE(sig.auth_method,''),COALESCE(sig.ip_address,''),
		COALESCE(sig.user_agent,''),COALESCE(sig.digital_signature_hash,''),COALESCE(sig.certificate_thumbprint,''),
		sig.signed_at,COALESCE(sig.evidence_bundle_uri,''),sig.created_at`, tenantID, sigID, authMethod, ipAddress, now, hash))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return sig, nil
}

func (s *postgresService) ListObligations(ctx context.Context, tenantID uuid.UUID, contractID *uuid.UUID) ([]*models.ContractObligation, error) {
	query := obligationSelect + ` JOIN contracts c ON c.id=o.contract_id WHERE c.tenant_id=$1`
	args := []any{tenantID}
	if contractID != nil {
		query += ` AND c.id=$2`
		args = append(args, *contractID)
	}
	rows, err := s.queryContext(ctx, tenantID, query+` ORDER BY o.due_date`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]*models.ContractObligation, 0)
	for rows.Next() {
		o, scanErr := scanObligation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

const obligationReturningColumns = `id,contract_id,COALESCE(title,''),COALESCE(category,''),
	COALESCE(party_responsible,''),COALESCE(owner_email,''),due_date,COALESCE(recurrence,''),
	COALESCE(status,''),COALESCE(penalty_details,''),COALESCE(service_credit_percent,0),
	COALESCE(evidence_notes,''),COALESCE(evidence_document_uri,''),completed_at,
	COALESCE(verified_by,''),created_at,updated_at`

const obligationReturningAliasColumns = `o.id,o.contract_id,COALESCE(o.title,''),COALESCE(o.category,''),
	COALESCE(o.party_responsible,''),COALESCE(o.owner_email,''),o.due_date,COALESCE(o.recurrence,''),
	COALESCE(o.status,''),COALESCE(o.penalty_details,''),COALESCE(o.service_credit_percent,0),
	COALESCE(o.evidence_notes,''),COALESCE(o.evidence_document_uri,''),o.completed_at,
	COALESCE(o.verified_by,''),o.created_at,o.updated_at`

const obligationSelect = `SELECT o.id,o.contract_id,COALESCE(o.title,''),COALESCE(o.category,''),
	COALESCE(o.party_responsible,''),COALESCE(o.owner_email,''),o.due_date,COALESCE(o.recurrence,''),
	COALESCE(o.status,''),COALESCE(o.penalty_details,''),COALESCE(o.service_credit_percent,0),
	COALESCE(o.evidence_notes,''),COALESCE(o.evidence_document_uri,''),o.completed_at,
	COALESCE(o.verified_by,''),o.created_at,o.updated_at FROM contract_obligations o`

func scanObligation(row interface{ Scan(...any) error }) (*models.ContractObligation, error) {
	o := new(models.ContractObligation)
	var completed sql.NullTime
	err := row.Scan(&o.ID,&o.ContractID,&o.Title,&o.Category,&o.PartyResponsible,&o.OwnerEmail,&o.DueDate,
		&o.Recurrence,&o.Status,&o.PenaltyDetails,&o.ServiceCreditPercent,&o.EvidenceNotes,
		&o.EvidenceDocumentURI,&completed,&o.VerifiedBy,&o.CreatedAt,&o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if completed.Valid {
		o.CompletedAt = &completed.Time
	}
	return o, nil
}

func (s *postgresService) AddObligation(ctx context.Context, tenantID, contractID uuid.UUID, o *models.ContractObligation) (*models.ContractObligation, error) {
	if o == nil {
		return nil, errors.New("obligation is required")
	}
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	now := time.Now().UTC()
	o.ContractID, o.Status, o.CreatedAt, o.UpdatedAt = contractID, "in_progress", now, now
	query := `INSERT INTO contract_obligations (id,contract_id,title,category,party_responsible,owner_email,due_date,
		recurrence,status,penalty_details,service_credit_percent,evidence_notes,evidence_document_uri,completed_at,
		verified_by,created_at,updated_at) SELECT $1,c.id,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
		FROM contracts c WHERE c.tenant_id=$2 AND c.id=$18 RETURNING ` + obligationReturningColumns
	return scanObligation(s.queryRowContext(ctx, tenantID, query, o.ID, tenantID, o.Title, nullableString(o.Category),
		nullableString(o.PartyResponsible), nullableString(o.OwnerEmail), o.DueDate, nullableString(o.Recurrence),
		o.Status, nullableString(o.PenaltyDetails), o.ServiceCreditPercent, nullableString(o.EvidenceNotes),
		nullableString(o.EvidenceDocumentURI), o.CompletedAt, nullableString(o.VerifiedBy), now, now, contractID))
}

func (s *postgresService) FulfillObligation(ctx context.Context, tenantID, oblID uuid.UUID, verifiedBy, evidenceNotes string) (*models.ContractObligation, error) {
	query := `UPDATE contract_obligations o SET status='fulfilled',completed_at=$3,verified_by=$4,evidence_notes=$5,updated_at=$3
		FROM contracts c WHERE c.id=o.contract_id AND c.tenant_id=$1 AND o.id=$2 RETURNING ` + obligationReturningAliasColumns
	o, err := scanObligation(s.queryRowContext(ctx, tenantID, query, tenantID, oblID, time.Now().UTC(), verifiedBy, evidenceNotes))
	return o, notFound(err)
}

func (s *postgresService) GetAnalytics(ctx context.Context, tenantID uuid.UUID) (*AnalyticsSummary, error) {
	summary := &AnalyticsSummary{TypeDistribution: make(map[string]int), RiskDistribution: make(map[string]int)}
	var compliant, cycleCount int
	var cycleDays float64
	err := s.queryRowContext(ctx, tenantID, `SELECT count(*),COALESCE(sum(total_value),0),
		count(*) FILTER (WHERE status='active'),count(*) FILTER (WHERE status='negotiation'),
		count(*) FILTER (WHERE status='ready_for_signature'),
		count(*) FILTER (WHERE expiration_date>now() AND expiration_date<now()+interval '90 days'),
		count(*) FILTER (WHERE compliance_status='compliant'),
		count(*) FILTER (WHERE effective_date IS NOT NULL),
		COALESCE(avg(EXTRACT(EPOCH FROM (effective_date-created_at))/86400) FILTER (WHERE effective_date IS NOT NULL),0)
		FROM contracts WHERE tenant_id=$1`, tenantID).Scan(&summary.TotalContracts, &summary.TotalContractValue,
		&summary.ActiveContracts, &summary.InNegotiation, &summary.PendingSignature,
		&summary.ExpiringWithin90Days, &compliant, &cycleCount, &cycleDays)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryContext(ctx, tenantID, `SELECT type,count(*),CASE WHEN risk_score>=70 THEN 'high' WHEN risk_score>=40 THEN 'medium' ELSE 'low' END
		FROM contracts WHERE tenant_id=$1 GROUP BY type,CASE WHEN risk_score>=70 THEN 'high' WHEN risk_score>=40 THEN 'medium' ELSE 'low' END`, tenantID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var typ, risk string
		var count int
		if err := rows.Scan(&typ, &count, &risk); err != nil {
			rows.Close()
			return nil, err
		}
		summary.TypeDistribution[typ] += count
		summary.RiskDistribution[risk] += count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if summary.TotalContracts > 0 {
		summary.ComplianceRate = float64(compliant) * 100 / float64(summary.TotalContracts)
	}
	if cycleCount > 0 {
		summary.AverageCycleDays = int(cycleDays)
	}
	return summary, nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	return err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func jsonValue(value, fallback any) string {
	if value == nil {
		value = fallback
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func hashContent(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func applyVariables(content string, variables map[string]string) string {
	for key, value := range variables {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	return content
}
