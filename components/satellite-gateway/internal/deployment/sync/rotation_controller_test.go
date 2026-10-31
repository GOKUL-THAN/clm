package sync

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestRotationController_EvaluateEstate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewRotationController(RotationPolicyConfig{
		RotationWindowDays: 30,
		WarningWindowDays:  60,
	}, logger)

	candidates := []RotationCandidate{
		{ID: "cert-01", Domain: "api.m2p.in", DaysLeft: 12, AutoRenew: true, SerialNumber: "1001"},
		{ID: "cert-02", Domain: "pay.m2p.in", DaysLeft: 25, AutoRenew: false, SerialNumber: "1002"},
		{ID: "cert-03", Domain: "auth.m2p.in", DaysLeft: 95, AutoRenew: true, SerialNumber: "1003"},
	}

	eligible, summary := ctrl.EvaluateEstate(candidates)

	if summary.TotalMonitored != 3 {
		t.Fatalf("expected 3 monitored, got %d", summary.TotalMonitored)
	}
	if summary.InRotationWindow != 2 {
		t.Fatalf("expected 2 in rotation window, got %d", summary.InRotationWindow)
	}
	if summary.AutoRenewEligible != 1 {
		t.Fatalf("expected 1 auto renew eligible, got %d", summary.AutoRenewEligible)
	}
	if len(eligible) != 1 || eligible[0].ID != "cert-01" {
		t.Fatalf("expected cert-01 eligible, got: %+v", eligible)
	}
}

func TestRotationController_SuccessfulRotation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewRotationController(RotationPolicyConfig{
		RotationWindowDays: 30,
		AutoRollbackOnFail: true,
	}, logger)

	cand := RotationCandidate{
		ID:           "cert-prod-01",
		Domain:       "secure.m2pfintech.com",
		DaysLeft:     15,
		SerialNumber: "old-serial-999",
	}

	deployCalled := false
	verifyCalled := false

	rec, err := ctrl.ExecuteRotation(
		context.Background(),
		cand,
		func() error {
			deployCalled = true
			return nil
		},
		func() bool {
			verifyCalled = true
			return true
		},
	)

	if err != nil {
		t.Fatalf("execute rotation error: %v", err)
	}
	if !deployCalled || !verifyCalled {
		t.Fatalf("expected deploy and verify called")
	}
	if rec.Stage != StageCompleted {
		t.Fatalf("expected completed stage, got: %s", rec.Stage)
	}
	if !rec.HealthVerified {
		t.Fatalf("expected health verified true")
	}
	if rec.RollbackOccurred {
		t.Fatalf("did not expect rollback")
	}

	status := ctrl.GetStatus()
	if len(status.RecentRecords) != 1 {
		t.Fatalf("expected 1 record in history, got %d", len(status.RecentRecords))
	}
}

func TestRotationController_FailedVerificationRollback(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewRotationController(RotationPolicyConfig{
		RotationWindowDays: 30,
		AutoRollbackOnFail: true,
	}, logger)

	cand := RotationCandidate{
		ID:           "cert-prod-failing",
		Domain:       "unstable.m2pfintech.com",
		DaysLeft:     8,
		SerialNumber: "serial-baseline-1",
	}

	rec, err := ctrl.ExecuteRotation(
		context.Background(),
		cand,
		func() error { return nil },
		func() bool { return false }, // verification fails!
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Stage != StageRolledBack {
		t.Fatalf("expected stage rolled_back, got: %s", rec.Stage)
	}
	if !rec.RollbackOccurred {
		t.Fatalf("expected rollback occurred true")
	}
	if rec.HealthVerified {
		t.Fatalf("expected health verified false")
	}
}

