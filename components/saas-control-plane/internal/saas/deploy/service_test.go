package deploy

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDeploymentJobTenantIsolation(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	jobID := uuid.New()
	service := &Service{jobs: map[uuid.UUID]*DeployJob{
		jobID: {
			ID:       jobID,
			TenantID: tenantA,
			State:    StatePending,
			Steps:    []JobStep{{Name: "validate", Status: "pending"}},
		},
	}}

	if _, err := service.GetJobForTenant(tenantB, jobID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("cross-tenant job read error = %v, want ErrJobNotFound", err)
	}
	if _, err := service.GetJobLogsForTenant(tenantB, jobID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("cross-tenant log read error = %v, want ErrJobNotFound", err)
	}
	if err := service.CancelJobForTenant(tenantB, jobID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("cross-tenant cancellation error = %v, want ErrJobNotFound", err)
	}
	if service.jobs[jobID].State != StatePending {
		t.Fatalf("cross-tenant cancellation changed state to %q", service.jobs[jobID].State)
	}

	jobs := service.ListJobs(tenantB)
	if len(jobs) != 0 {
		t.Fatalf("cross-tenant list returned %d jobs", len(jobs))
	}
}
