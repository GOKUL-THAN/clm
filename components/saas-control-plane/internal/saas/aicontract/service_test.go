package aicontract_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/saas/aicontract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAIContractIntelligence(t *testing.T) {
	ctx := context.Background()
	engine := aicontract.NewEngine(zap.NewNop())
	tenantID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	contractID := uuid.New()

	sampleText := `This Master Agreement includes mutual confidentiality for 3 years.
Neither party shall be liable for indirect or consequential damages, and liability is capped at 12 months fees.
Service provider guarantees 99.99% SLA availability and annual SOC 2 Type II audit report delivery.`

	// 1. Analyze
	analysis, err := engine.AnalyzeContract(ctx, tenantID, contractID, sampleText)
	require.NoError(t, err)
	assert.NotEmpty(t, analysis.ID)
	assert.Greater(t, analysis.ConfidenceScore, 0.90)
	assert.NotEmpty(t, analysis.ExtractedClauses)
	assert.NotEmpty(t, analysis.ExtractedObligations)

	// 2. Q&A
	ans, err := engine.AskAssistant(ctx, tenantID, contractID, "What is the liability cap?")
	require.NoError(t, err)
	assert.Contains(t, ans.Answer, "capped")
	assert.NotEmpty(t, ans.CitedClauses)

	// 3. Compare contracts
	comp, err := engine.CompareContracts(ctx, "Contract Alpha", "Contract Beta")
	require.NoError(t, err)
	assert.Greater(t, comp.SimilarityScore, 0.8)
	assert.NotEmpty(t, comp.KeyDifferences)
}

