package itsm

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap/zaptest"
)

func TestITSMController_IncidentLifecycle(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewITSMController(logger)
	tenantID := uuid.New()
	ctx := context.Background()

	// 1. Raise SEV1 incident
	inc, err := ctrl.RaiseIncident(ctx, tenantID, &Incident{
		Severity:       SeveritySEV1,
		Title:          "Production TLS Expiry Outage on Checkout API",
		AffectedDomain: "api-checkout.m2pfintech.com",
	})
	if err != nil {
		t.Fatalf("failed to raise incident: %v", err)
	}

	if inc.BlastRadiusScore != 95 {
		t.Fatalf("expected blast radius 95, got %d", inc.BlastRadiusScore)
	}
	if inc.Status != StatusOpen {
		t.Fatalf("expected open status, got %s", inc.Status)
	}

	// 2. List
	list := ctrl.ListIncidents(tenantID)
	if len(list) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(list))
	}

	// 3. Resolve incident
	resolved, err := ctrl.ResolveIncident(ctx, inc.ID, "Automated renewal token expired", "Renewed via DigiCert ACME with zero-downtime staging")
	if err != nil {
		t.Fatalf("failed to resolve incident: %v", err)
	}
	if resolved.Status != StatusResolved {
		t.Fatalf("expected resolved status")
	}
	if resolved.ResolvedAt == nil {
		t.Fatalf("expected resolved_at timestamp")
	}

	// 4. Try resolve again (should fail)
	_, err = ctrl.ResolveIncident(ctx, inc.ID, "duplicate", "duplicate")
	if err != ErrAlreadyResolved {
		t.Fatalf("expected ErrAlreadyResolved, got: %v", err)
	}
}

func TestITSMController_ServiceCatalogRequest(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctrl := NewITSMController(logger)
	tenantID := uuid.New()
	ctx := context.Background()

	req, err := ctrl.SubmitServiceRequest(ctx, tenantID, &ServiceRequest{
		RequesterEmail: "devops-lead@m2pfintech.com",
		Department:     "Core Banking",
		DomainName:     "internal-transfer.m2p.in",
		Environment:    "prod",
		CertType:       "ov",
		ValidityMonths: 12,
	})
	if err != nil {
		t.Fatalf("failed to submit service request: %v", err)
	}

	if req.Status != "submitted" {
		t.Fatalf("expected submitted status, got %s", req.Status)
	}
	if req.ITSMTicketRef == "" {
		t.Fatalf("expected generated ticket ref")
	}

	reqs := ctrl.ListServiceRequests(tenantID)
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request in list, got %d", len(reqs))
	}
}
