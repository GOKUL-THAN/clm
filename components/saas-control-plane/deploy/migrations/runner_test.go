package migrations

import (
	"fmt"
	"regexp"
	"testing"
)

func TestListReturnsOrderedApplicationMigrations(t *testing.T) {
	got, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 11 {
		t.Fatalf("migration count = %d, want 11", len(got))
	}

	checksumPattern := regexp.MustCompile(`^[a-f0-9]{64}$`)
	for i, migration := range got {
		if want := fmt.Sprintf("%03d", i+1); migration.Version != want {
			t.Fatalf("migration %d version = %q, want %q", i, migration.Version, want)
		}
		if len(migration.Contents) == 0 {
			t.Errorf("migration %s has no SQL", migration.Filename)
		}
		if !checksumPattern.MatchString(migration.Checksum) {
			t.Errorf("migration %s has invalid checksum %q", migration.Filename, migration.Checksum)
		}
		if migration.Filename == "000_keycloak_db.sql" {
			t.Fatal("Keycloak database bootstrap must not be part of the CLM schema runner")
		}
	}
}
