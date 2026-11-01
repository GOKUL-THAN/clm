package contract_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/saas/contract"
	"github.com/m2p/clm/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestContractLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := contract.NewService(zap.NewNop())
	tenantID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")

	// 1. Create contract
	exp := time.Now().Add(180 * 24 * time.Hour)
	newContract := &models.Contract{
		Title:            "Vendor SaaS Licensing Agreement",
		Type:             models.ContractTypeLicense,
		CounterpartyName: "DataCloud Global Inc",
		TotalValue:       48000.00,
		Currency:         "USD",
		ExpirationDate:   &exp,
		ContentBody:      "Standard SaaS subscription terms with 99.9% uptime SLA.",
	}

	created, err := svc.CreateContract(ctx, tenantID, newContract)
	require.NoError(t, err)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, models.StatusDraft, created.Status)
	assert.Equal(t, 1, created.CurrentVersion)

	// 2. Lifecycle transitions
	transitioned, err := svc.TransitionContract(ctx, tenantID, created.ID, models.StatusNegotiation, models.StageNegotiation, "Sent to vendor for redline")
	require.NoError(t, err)
	assert.Equal(t, models.StatusNegotiation, transitioned.Status)

	// 3. Versions
	v2, err := svc.CreateVersion(ctx, tenantID, created.ID, "Vendor SaaS Agreement v2", "Updated SLA to 99.99%", "Negotiation round 1 adjustments", "legal@enterprise.internal")
	require.NoError(t, err)
	assert.Equal(t, 2, v2.VersionNumber)

	versions, err := svc.ListVersions(ctx, tenantID, created.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 2)

	// 4. Analytics
	analytics, err := svc.GetAnalytics(ctx, tenantID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, analytics.TotalContracts, 1)
}

func TestIntakeAndAssembly(t *testing.T) {
	ctx := context.Background()
	svc := contract.NewService(zap.NewNop())
	tenantID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")

	// Submit intake request
	req := &models.ContractRequest{
		Title:                 "Procurement NDA for Hardware HSMs",
		ContractType:          "NDA",
		CounterpartyName:      "SecureHardware Corp",
		RequesterEmail:        "secops@enterprise.internal",
		EstimatedValue:        0.0,
		BusinessJustification: "Evaluating tamper-resistant cryptographic accelerator modules",
	}

	submitted, err := svc.SubmitRequest(ctx, tenantID, req)
	require.NoError(t, err)
	assert.Equal(t, "submitted", submitted.Status)

	// Convert request
	converted, err := svc.ConvertRequestToContract(ctx, tenantID, submitted.ID)
	require.NoError(t, err)
	assert.Equal(t, "Procurement NDA for Hardware HSMs", converted.Title)
	assert.Equal(t, models.StatusDraft, converted.Status)

	// Clauses
	clauses, err := svc.ListClauses(ctx, tenantID, "Confidentiality")
	require.NoError(t, err)
	assert.NotEmpty(t, clauses)
}

