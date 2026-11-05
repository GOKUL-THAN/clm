package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m2p/clm/internal/core"
	"github.com/m2p/clm/pkg/models"
)

func TestCertificatePaginationRejectsOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if _, _, _, err := certificatePagination(&core.CertFilter{Page: maxInt, PageSize: 2}); err == nil {
		t.Fatal("expected overflowing certificate pagination to be rejected")
	}

	page, pageSize, offset, err := certificatePagination(&core.CertFilter{Page: 3, PageSize: 10})
	if err != nil {
		t.Fatalf("valid certificate pagination: %v", err)
	}
	if page != 3 || pageSize != 10 || offset != 20 {
		t.Fatalf("pagination = page %d, size %d, offset %d; want page 3, size 10, offset 20", page, pageSize, offset)
	}
	if _, _, _, err := certificatePagination(nil); err != nil {
		t.Fatalf("nil certificate filter: %v", err)
	}
}

func TestCertificateRepositoryListsAcrossTenantsWithScopedQueries(t *testing.T) {
	skipIfNoDB(t)
	ctx := context.Background()
	tenantA := seedTenant(t, ctx, "cert-scan-a-"+uuid.NewString()[:8])
	tenantB := seedTenant(t, ctx, "cert-scan-b-"+uuid.NewString()[:8])
	repo := NewCertificateRepository(testDB)
	const environment = "repository-tenant-scan-test"

	createdByA := uuid.New()
	createdByB := uuid.New()
	for _, requester := range []struct {
		id       uuid.UUID
		tenantID uuid.UUID
	}{
		{id: createdByA, tenantID: tenantA},
		{id: createdByB, tenantID: tenantB},
	} {
		_, err := testDB.ExecContext(ctx,
			`INSERT INTO users (id, tenant_id, email, name, role, status, password_hash)
			 VALUES ($1, $2, $3, 'Certificate Test User', 'approver', 'active', 'test-only')`,
			requester.id, requester.tenantID, uuid.NewString()+"@example.invalid")
		if err != nil {
			t.Fatalf("seed certificate requester: %v", err)
		}
	}

	createdAtA := time.Now().UTC()
	createdAtB := createdAtA.Add(time.Minute)
	certA := &models.Certificate{
		ID: uuid.New(), TenantID: tenantA, Domain: "tenant-a.example.invalid",
		Type: models.CertTypeDV, Status: models.CertStatusActive,
		Environment: environment, CreatedBy: createdByA, CreatedAt: createdAtA, UpdatedAt: createdAtA,
	}
	certB := &models.Certificate{
		ID: uuid.New(), TenantID: tenantB, Domain: "tenant-b.example.invalid",
		Type: models.CertTypeDV, Status: models.CertStatusActive,
		Environment: environment, CreatedBy: createdByB, CreatedAt: createdAtB, UpdatedAt: createdAtB,
	}
	t.Cleanup(func() {
		_ = repo.Delete(context.Background(), tenantA, certA.ID)
		_ = repo.Delete(context.Background(), tenantB, certB.ID)
	})
	for _, cert := range []*models.Certificate{certA, certB} {
		if err := repo.Create(ctx, cert); err != nil {
			t.Fatalf("create tenant certificate: %v", err)
		}
	}

	ownCertificates, ownTotal, err := repo.List(ctx, tenantA, &core.CertFilter{Environment: environment})
	if err != nil {
		t.Fatalf("list owning tenant certificates: %v", err)
	}
	if ownTotal != 1 || len(ownCertificates) != 1 || ownCertificates[0].ID != certA.ID {
		t.Fatalf("own tenant list = %d rows, total %d; want certificate %s only", len(ownCertificates), ownTotal, certA.ID)
	}

	if _, _, err := repo.List(ctx, uuid.Nil, &core.CertFilter{Environment: environment}); err == nil {
		t.Fatal("tenant-scoped list accepted a nil tenant")
	}

	firstPage, total, err := repo.ListAllTenants(ctx, &core.CertFilter{Environment: environment, Page: 1, PageSize: 1})
	if err != nil {
		t.Fatalf("list certificates across tenants: %v", err)
	}
	if total != 2 || len(firstPage) != 1 || firstPage[0].ID != certB.ID {
		t.Fatalf("global page 1 = %v, total %d; want newest certificate %s and total 2", certificateIDs(firstPage), total, certB.ID)
	}
	secondPage, secondTotal, err := repo.ListAllTenants(ctx, &core.CertFilter{Environment: environment, Page: 2, PageSize: 1})
	if err != nil {
		t.Fatalf("list second certificate page across tenants: %v", err)
	}
	if secondTotal != total || len(secondPage) != 1 || secondPage[0].ID != certA.ID {
		t.Fatalf("global page 2 = %v, total %d; want certificate %s and total 2", certificateIDs(secondPage), secondTotal, certA.ID)
	}
}

func certificateIDs(certificates []*models.Certificate) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(certificates))
	for _, cert := range certificates {
		ids = append(ids, cert.ID)
	}
	return ids
}
