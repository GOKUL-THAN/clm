package dns01

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	name       string
	presentErr error
	cleanupErr error
	presented  []Record
	cleaned    []Record
}

func (f *fakeProvider) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}
func (f *fakeProvider) Present(_ context.Context, record Record) error {
	f.presented = append(f.presented, record)
	return f.presentErr
}
func (f *fakeProvider) Cleanup(_ context.Context, record Record) error {
	f.cleaned = append(f.cleaned, record)
	return f.cleanupErr
}

type fakeResolver struct {
	values []string
	err    error
	calls  int
}

func (f *fakeResolver) LookupTXT(_ context.Context, _ string) ([]string, error) {
	f.calls++
	return append([]string(nil), f.values...), f.err
}

func TestProviderContractSolve(t *testing.T) {
	provider := &fakeProvider{}
	resolver := &fakeResolver{values: []string{"challenge-value"}}
	solver := NewSolver(provider, resolver, Config{
		TTL: 90, PollInterval: time.Millisecond, PropagationTimeout: time.Second,
	})

	validated := false
	err := solver.Solve(context.Background(), "api.example.com", "challenge-value", func(context.Context) error {
		validated = true
		return nil
	})
	if err != nil {
		t.Fatalf("Solve() error = %v", err)
	}
	if !validated {
		t.Fatal("validation callback was not called")
	}
	if len(provider.presented) != 1 || len(provider.cleaned) != 1 {
		t.Fatalf("provider calls: presented=%d cleaned=%d", len(provider.presented), len(provider.cleaned))
	}
	want := Record{FQDN: "_acme-challenge.api.example.com.", Value: "challenge-value", TTL: 90}
	if provider.presented[0] != want || provider.cleaned[0] != want {
		t.Fatalf("provider records = %#v / %#v, want %#v", provider.presented[0], provider.cleaned[0], want)
	}
}

func TestRecordName(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    bool
	}{
		{name: "ordinary", identifier: "api.example.com", want: "_acme-challenge.api.example.com."},
		{name: "wildcard uses apex", identifier: "*.Example.COM", want: "_acme-challenge.example.com."},
		{name: "trailing dot", identifier: "example.com.", want: "_acme-challenge.example.com."},
		{name: "embedded wildcard", identifier: "api.*.example.com", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RecordName(tt.identifier)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RecordName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("RecordName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanupOnAllErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		presentErr error
		resolver   *fakeResolver
		validate   func(context.Context) error
	}{
		{
			name: "present error can follow partial mutation", presentErr: errors.New("response lost"),
			resolver: &fakeResolver{values: []string{"value"}},
			validate: func(context.Context) error { return nil },
		},
		{
			name: "validation error", resolver: &fakeResolver{values: []string{"value"}},
			validate: func(context.Context) error { return errors.New("CA rejected challenge") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeProvider{presentErr: tt.presentErr}
			solver := NewSolver(provider, tt.resolver, Config{
				PollInterval: time.Millisecond, PropagationTimeout: 50 * time.Millisecond,
			})
			if err := solver.Solve(context.Background(), "example.com", "value", tt.validate); err == nil {
				t.Fatal("Solve() error = nil")
			}
			if len(provider.cleaned) != 1 {
				t.Fatalf("Cleanup calls = %d, want 1", len(provider.cleaned))
			}
		})
	}
}

func TestPropagationTimeoutCleansUp(t *testing.T) {
	provider := &fakeProvider{}
	resolver := &fakeResolver{err: errors.New("NXDOMAIN")}
	solver := NewSolver(provider, resolver, Config{
		PollInterval: 2 * time.Millisecond, PropagationTimeout: 12 * time.Millisecond,
	})

	err := solver.Solve(context.Background(), "example.com", "never-visible", func(context.Context) error {
		t.Fatal("validation must not run before propagation")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Solve() error = %v, want propagation timeout", err)
	}
	if resolver.calls < 1 {
		t.Fatal("resolver was not polled")
	}
	if len(provider.cleaned) != 1 {
		t.Fatalf("Cleanup calls = %d, want 1", len(provider.cleaned))
	}
}

func TestAdapterCatalogMarksImplementedProvidersLive(t *testing.T) {
	for _, adapter := range Adapters() {
		wantLive := adapter.Name == ProviderRFC2136 || adapter.Name == ProviderRoute53
		if adapter.Live != wantLive {
			t.Fatalf("adapter %q live=%v, want %v", adapter.Name, adapter.Live, wantLive)
		}
		if !adapter.Live && !strings.Contains(adapter.Notes, "planned") {
			t.Fatalf("planned adapter %q has ambiguous notes %q", adapter.Name, adapter.Notes)
		}
	}
}

func TestRFC2136CleanupDeletesOnlyChallengeValue(t *testing.T) {
	var script string
	provider := &RFC2136Provider{
		zone: "example.com.", nameserver: "192.0.2.53 53", keyPath: "/unused",
		run: func(_ context.Context, value string) error {
			script = value
			return nil
		},
	}
	record := Record{FQDN: "_acme-challenge.example.com.", Value: "token-value", TTL: 60}
	if err := provider.Cleanup(context.Background(), record); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if !strings.Contains(script, "update delete _acme-challenge.example.com. TXT \"token-value\"") {
		t.Fatalf("cleanup script deletes more than the challenge value: %q", script)
	}
}

func TestPlannedCloudAdaptersFailBeforeIssuance(t *testing.T) {
	for _, provider := range []string{ProviderAzure, ProviderGCP, ProviderOCI} {
		t.Run(provider, func(t *testing.T) {
			_, err := New(Config{Provider: provider})
			if err == nil || !strings.Contains(err.Error(), "planned but not live") {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestCredentialFileRequiresReference(t *testing.T) {
	if _, err := credentialFile("raw-tsig-secret"); err == nil {
		t.Fatal("credentialFile accepted raw key material")
	}
	expectedPath := filepath.Join(t.TempDir(), "acme.key")
	path, err := credentialFile("file-ref://" + expectedPath)
	if err != nil {
		t.Fatalf("credentialFile() error = %v", err)
	}
	if path != expectedPath {
		t.Fatalf("credentialFile() = %q", path)
	}
}
