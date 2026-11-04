package contract

import (
	"database/sql"
	"testing"
)

func TestNewPostgresServiceRequiresDatabase(t *testing.T) {
	if _, err := NewPostgresService((*sql.DB)(nil), nil); err == nil {
		t.Fatal("NewPostgresService() accepted a nil database")
	}
}

func TestEscapeLike(t *testing.T) {
	got := escapeLike(`50%_off\special`)
	want := `50\%\_off\\special`
	if got != want {
		t.Fatalf("escapeLike() = %q, want %q", got, want)
	}
}

func TestApplyVariables(t *testing.T) {
	got := applyVariables("Agreement: {{PartyA}} and {{PartyB}}", map[string]string{
		"PartyA": "Acme", "PartyB": "Example LLC",
	})
	want := "Agreement: Acme and Example LLC"
	if got != want {
		t.Fatalf("applyVariables() = %q, want %q", got, want)
	}
}

func TestHashContent(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := hashContent("abc"); got != want {
		t.Fatalf("hashContent() = %q, want %q", got, want)
	}
}
