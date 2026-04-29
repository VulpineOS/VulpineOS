package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempTLSHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})
	return tmpHome
}

func requireTLSMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func TestGenerateSelfSignedCertUsesPrivateKeyPermissions(t *testing.T) {
	home := withTempTLSHome(t)

	certPath, keyPath, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}

	requireTLSMode(t, filepath.Join(home, ".vulpineos", "tls"), 0700)
	requireTLSMode(t, certPath, 0644)
	requireTLSMode(t, keyPath, 0600)
}

func TestGenerateSelfSignedCertRepairsExistingKeyPermissions(t *testing.T) {
	home := withTempTLSHome(t)
	tlsDir := filepath.Join(home, ".vulpineos", "tls")
	if err := os.MkdirAll(tlsDir, 0755); err != nil {
		t.Fatalf("mkdir tls dir: %v", err)
	}
	certPath := filepath.Join(tlsDir, "vulpineos.crt")
	keyPath := filepath.Join(tlsDir, "vulpineos.key")
	if err := os.WriteFile(certPath, []byte("cert"), 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("key"), 0644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	gotCert, gotKey, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert: %v", err)
	}
	if gotCert != certPath || gotKey != keyPath {
		t.Fatalf("paths = %q, %q; want %q, %q", gotCert, gotKey, certPath, keyPath)
	}

	requireTLSMode(t, tlsDir, 0700)
	requireTLSMode(t, certPath, 0644)
	requireTLSMode(t, keyPath, 0600)
}
