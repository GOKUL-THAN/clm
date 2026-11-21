package api

import "testing"

func TestTenantScopedCertificates(t *testing.T) {
	certs := []*Certificate{
		{ID: "cert-a", TenantID: "tenant-a"},
		{ID: "cert-b", TenantID: "tenant-b"},
	}

	scoped, err := tenantScopedCertificates(certs, "tenant-a", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].ID != "cert-a" {
		t.Fatalf("tenant-a scoped inventory = %+v", scoped)
	}

	if _, err := tenantScopedCertificates(certs, "", false); err == nil {
		t.Fatal("missing tenant context was accepted")
	}

	all, err := tenantScopedCertificates(certs, "", true)
	if err != nil || len(all) != 2 {
		t.Fatalf("global admin inventory = %d, err=%v", len(all), err)
	}
}
