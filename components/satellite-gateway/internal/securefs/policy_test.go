package securefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPolicyAllowsOnlyConfiguredRoots(t *testing.T) {
	requireLinuxSecuritySemantics(t)
	approved := t.TempDir()
	outside := t.TempDir()
	policy, err := NewPolicy([]string{approved}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = policy.Close() })

	path := filepath.Join(approved, "cert.pem")
	if err := policy.WriteFile(path, "", Certificate, []byte("public cert"), false); err != nil {
		t.Fatalf("write in approved root: %v", err)
	}
	if _, err := policy.ReadFile(path, "", Certificate); err != nil {
		t.Fatalf("read in approved root: %v", err)
	}
	if err := policy.WriteFile(filepath.Join(outside, "cert.pem"), "", Certificate, []byte("outside"), false); err == nil {
		t.Fatal("write outside approved root unexpectedly succeeded")
	}
}

func TestPolicyRejectsTraversalAndEncodedPaths(t *testing.T) {
	requireLinuxSecuritySemantics(t)
	root := t.TempDir()
	policy, err := NewPolicy([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = policy.Close() })

	for _, path := range []string{
		root + string(filepath.Separator) + ".." + string(filepath.Separator) + "outside.pem",
		filepath.Join(root, "%2e%2e", "outside.pem"),
		filepath.Join(root, "bad\x00.pem"),
		"relative.pem",
	} {
		if err := policy.WriteFile(path, "", Certificate, []byte("x"), false); err == nil {
			t.Errorf("WriteFile(%q) unexpectedly succeeded", path)
		}
	}
}

func TestPolicyRequiresExplicitReplacementAndProtectsPrivateKeys(t *testing.T) {
	requireLinuxSecuritySemantics(t)
	root := t.TempDir()
	policy, err := NewPolicy([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = policy.Close() })

	path := filepath.Join(root, "private.key")
	if err := policy.WriteFile(path, "", PrivateKey, []byte("first"), false); err != nil {
		t.Fatal(err)
	}
	if err := policy.WriteFile(path, "", PrivateKey, []byte("second"), false); err == nil {
		t.Fatal("replacement without authorization unexpectedly succeeded")
	}
	if err := policy.WriteFile(path, "", PrivateKey, []byte("second"), true); err != nil {
		t.Fatalf("authorized replacement: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private-key mode = %04o, want 0600", info.Mode().Perm())
	}
	if _, err := policy.ReadFile(path, "", PrivateKey); err != nil {
		t.Fatalf("read private key: %v", err)
	}
}

func TestPolicyUsesTenantSpecificRoots(t *testing.T) {
	requireLinuxSecuritySemantics(t)
	tenantRoot := t.TempDir()
	otherRoot := t.TempDir()
	policy, err := NewPolicy(nil, map[string][]string{"tenant-1": {tenantRoot}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = policy.Close() })

	allowed := filepath.Join(tenantRoot, "certificate.pem")
	if err := policy.WriteFile(allowed, "tenant-1", Certificate, []byte("cert"), false); err != nil {
		t.Fatal(err)
	}
	if err := policy.WriteFile(filepath.Join(otherRoot, "certificate.pem"), "tenant-1", Certificate, []byte("cert"), false); err == nil {
		t.Fatal("tenant wrote outside its configured root")
	}
	if err := policy.WriteFile(allowed, "tenant-2", Certificate, []byte("cert"), false); err == nil {
		t.Fatal("tenant without an explicit root unexpectedly used another tenant's root")
	}
}

func TestPolicyRejectsSymbolicLinkEscape(t *testing.T) {
	requireLinuxSecuritySemantics(t)
	rootDir := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "private.key")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootDir, "escape")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	policy, err := NewPolicy([]string{rootDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = policy.Close() })
	if _, err := policy.ReadFile(link, "", PrivateKey); err == nil {
		t.Fatal("read through symlink escape unexpectedly succeeded")
	}
}

func TestPolicyRejectsWritableRoot(t *testing.T) {
	requireLinuxSecuritySemantics(t)
	root := t.TempDir()
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPolicy([]string{root}, nil); err == nil {
		t.Fatal("world-writable approved root unexpectedly accepted")
	}
}

func TestPolicyDoesNotCreateApprovedRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, err := NewPolicy([]string{root}, nil); err == nil {
		t.Fatal("nonexistent approved root unexpectedly accepted")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("approved root was created unexpectedly; stat error = %v", err)
	}
}

func requireLinuxSecuritySemantics(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("filesystem ownership and mode checks are supported only on Linux")
	}
}
