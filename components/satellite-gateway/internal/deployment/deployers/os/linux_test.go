package osdeployer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/m2p/clm/internal/satellite/deployment/deployers"
	"github.com/m2p/clm/internal/satellite/securefs"
	"go.uber.org/zap"
)

func TestSystemctlCommand(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		reloadCmd string
		want      string
		wantErr   bool
	}{
		{name: "default reload", service: "nginx.service", want: "reload"},
		{name: "explicit restart", service: "nginx", reloadCmd: "systemctl restart nginx", want: "restart"},
		{name: "reject shell command", service: "nginx", reloadCmd: "sh -c id", wantErr: true},
		{name: "reject shell metacharacters", service: "nginx", reloadCmd: "systemctl reload nginx; id", wantErr: true},
		{name: "reject other service", service: "nginx", reloadCmd: "systemctl reload sshd", wantErr: true},
		{name: "reject unsafe service name", service: "nginx;id", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, service, err := systemctlCommand(tt.service, tt.reloadCmd)
			if (err != nil) != tt.wantErr {
				t.Fatalf("systemctlCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (action != tt.want || service != tt.service) {
				t.Fatalf("systemctlCommand() = (%q, %q), want (%q, %q)", action, service, tt.want, tt.service)
			}
		})
	}
}

func TestLinuxDeployerEnforcesTenantRootsAndReplacementAuthorization(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("secure filesystem deployment tests require Linux ownership and mode semantics")
	}
	rootA := t.TempDir()
	rootB := t.TempDir()
	files, err := securefs.NewPolicy(nil, map[string][]string{
		"tenant-a": {rootA},
		"tenant-b": {rootB},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = files.Close() })

	deployer := NewLinuxDeployer(zap.NewNop(), files)
	target := &deployers.DeployTarget{
		TenantID: "tenant-a",
		CertPath: filepath.Join(rootA, "cert.pem"),
		KeyPath:  filepath.Join(rootA, "key.pem"),
	}
	request := &deployers.DeployRequest{
		CertPEM: []byte("certificate-v1"), KeyPEM: []byte("private-key-v1"),
		Target: target,
	}
	if _, err := deployer.Deploy(context.Background(), request); err != nil {
		t.Fatalf("initial deployment: %v", err)
	}

	request.CertPEM = []byte("certificate-v2")
	request.KeyPEM = []byte("private-key-v2")
	if _, err := deployer.Deploy(context.Background(), request); err == nil {
		t.Fatal("replacement without API authorization unexpectedly succeeded")
	}
	current, err := os.ReadFile(target.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "certificate-v1" {
		t.Fatalf("unauthorized replacement changed certificate to %q", current)
	}

	target.AllowReplace = true
	if _, err := deployer.Deploy(context.Background(), request); err != nil {
		t.Fatalf("authorized replacement: %v", err)
	}
	current, err = os.ReadFile(target.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "certificate-v2" {
		t.Fatalf("authorized replacement left certificate %q", current)
	}

	target.CertPath = filepath.Join(rootB, "escape.pem")
	if _, err := deployer.Deploy(context.Background(), request); err == nil {
		t.Fatal("tenant-a deployment into tenant-b root unexpectedly succeeded")
	}
}

func TestLinuxDeployerFailsClosedWithoutPolicy(t *testing.T) {
	deployer := NewLinuxDeployer(zap.NewNop(), nil)
	_, err := deployer.Deploy(context.Background(), &deployers.DeployRequest{
		CertPEM: []byte("cert"), KeyPEM: []byte("key"),
		Target: &deployers.DeployTarget{TenantID: "tenant-a"},
	})
	if err == nil {
		t.Fatal("deployment without a file policy unexpectedly succeeded")
	}
}
