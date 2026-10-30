package deployment

import (
	"context"
	"encoding/pem"
	"errors"
	"net"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"go.uber.org/zap"
)

type fakeDeployer struct {
	name          string
	category      string
	deployCalls   int
	verifyCalls   int
	rollbackCalls int
	adapterOK     bool
	rollbackErr   error
}

func (f *fakeDeployer) Name() string     { return f.name }
func (f *fakeDeployer) Category() string { return f.category }
func (f *fakeDeployer) Deploy(ctx context.Context, req *deployers.DeployRequest) (*deployers.DeployResult, error) {
	f.deployCalls++
	return &deployers.DeployResult{Success: true, Target: req.Target}, nil
}
func (f *fakeDeployer) Verify(ctx context.Context, req *deployers.DeployRequest, applied *deployers.DeployResult) (*deployers.VerificationEvidence, error) {
	f.verifyCalls++
	evidence := &deployers.VerificationEvidence{
		Kind: "adapter", Method: "fake-api", Adapter: f.name, Success: f.adapterOK,
	}
	if !f.adapterOK {
		return evidence, errors.New("fake adapter verification failed")
	}
	return evidence, nil
}
func (f *fakeDeployer) Rollback(ctx context.Context, target *deployers.DeployTarget) error {
	f.rollbackCalls++
	return f.rollbackErr
}

func TestExecutorPreflightRejectsAWSListenerWithoutServingHost(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	fake := &fakeDeployer{name: "aws-acm", category: "cloud", adapterOK: true}
	executor := NewExecutor(zap.NewNop())
	executor.RegisterDeployer(fake)

	_, err := executor.Deploy(context.Background(), &deployers.DeployRequest{
		CertPEM: certPEM,
		Target: &deployers.DeployTarget{
			Category: "cloud", Type: "aws-acm", ApplyTo: "acm+alb",
			ListenerARN: "arn:aws:elasticloadbalancing:region:account:listener/app/name/id/id",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "serving_host") {
		t.Fatalf("err=%v, want serving_host preflight failure", err)
	}
	if fake.deployCalls != 0 {
		t.Fatalf("Deploy called %d times despite failed preflight", fake.deployCalls)
	}
}

func TestExecutorEndpointEvidenceCannotBeSubstitutedByAdapter(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	host, port := serverAddress(t, server.URL)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	fake := &fakeDeployer{name: "custom", category: "app", adapterOK: true}
	executor := NewExecutor(zap.NewNop())
	executor.RegisterDeployer(fake)

	result, err := executor.Deploy(context.Background(), &deployers.DeployRequest{
		CertPEM: certPEM,
		Target: &deployers.DeployTarget{
			Category: "app", Type: "custom", ServingHost: host, ServingPort: port, ServerName: host,
		},
	})
	if err == nil || result == nil || result.Verified {
		t.Fatalf("result=%+v err=%v, want endpoint verification failure", result, err)
	}
	if fake.verifyCalls != 1 || len(result.Evidence) != 2 || result.Evidence[0].Kind != "adapter" || result.Evidence[1].Kind != "endpoint" {
		t.Fatalf("unexpected verification calls/evidence: calls=%d evidence=%+v", fake.verifyCalls, result.Evidence)
	}
	if !result.Evidence[0].Success || result.Evidence[1].Success {
		t.Fatalf("adapter should pass while endpoint controls failure: %+v", result.Evidence)
	}
}

func TestExecutorAcceptsExplicitNonNetworkAdapterEvidence(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	fake := &fakeDeployer{name: "aws-acm", category: "cloud", adapterOK: true}
	executor := NewExecutor(zap.NewNop())
	executor.RegisterDeployer(fake)

	result, err := executor.Deploy(context.Background(), &deployers.DeployRequest{
		CertPEM: certPEM,
		Target:  &deployers.DeployTarget{Category: "cloud", Type: "aws-acm", ApplyTo: "acm"},
	})
	if err != nil || result == nil || !result.Success || !result.Verified {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Kind != "adapter" || !result.Evidence[0].Success {
		t.Fatalf("expected explicit adapter-only evidence, got %+v", result.Evidence)
	}
}

func serverAddress(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
