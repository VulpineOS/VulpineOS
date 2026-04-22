package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_VersionFlag verifies the Run wrapper-friendly entrypoint
// honors --version, writes a version string to the configured stdout,
// and returns exit code 0. This is the contract that an alternate
// front-end binary depends on when delegating to Run(os.Args).
func TestRun_VersionFlag(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	prevOut, prevErr := stdout, stderr
	stdout = &outBuf
	stderr = &errBuf
	t.Cleanup(func() {
		stdout = prevOut
		stderr = prevErr
	})

	code := Run([]string{"vulpineos", "--version"})
	if code != 0 {
		t.Fatalf("Run(--version) exit code = %d, want 0 (stderr=%q)", code, errBuf.String())
	}
	out := outBuf.String()
	if !strings.Contains(out, "vulpineos") {
		t.Errorf("Run(--version) stdout = %q, expected to contain %q", out, "vulpineos")
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("Run(--version) produced empty stdout")
	}
}

func TestStartLocalSessionLoggingWritesToFile(t *testing.T) {
	restore, path := startLocalSessionLogging(t.TempDir())
	if path == "" {
		t.Fatal("startLocalSessionLogging returned empty path")
	}

	log.Print("startup log redirected")
	restore()

	if filepath.Base(path) != "local-tui.log" {
		t.Fatalf("log path = %q, want local-tui.log", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "startup log redirected") {
		t.Fatalf("log file %q missing redirected message: %q", path, string(data))
	}
}

func TestNormalizeRemotePanelURL(t *testing.T) {
	got, err := normalizeRemotePanelURL("wss://example.com:8443/ws", "secret")
	if err != nil {
		t.Fatalf("normalizeRemotePanelURL error: %v", err)
	}
	want := "https://example.com:8443/?token=secret"
	if got != want {
		t.Fatalf("normalizeRemotePanelURL = %q, want %q", got, want)
	}
}

func TestNormalizeRemotePanelURL_DefaultLocalURL(t *testing.T) {
	got, err := normalizeRemotePanelURL("", "secret")
	if err != nil {
		t.Fatalf("normalizeRemotePanelURL default error: %v", err)
	}
	want := "http://127.0.0.1:8443/?token=secret"
	if got != want {
		t.Fatalf("normalizeRemotePanelURL default = %q, want %q", got, want)
	}
}

func TestNormalizeRemoteTUIURL(t *testing.T) {
	got, err := normalizeRemoteTUIURL("https://example.com:8443")
	if err != nil {
		t.Fatalf("normalizeRemoteTUIURL error: %v", err)
	}
	want := "wss://example.com:8443/ws"
	if got != want {
		t.Fatalf("normalizeRemoteTUIURL = %q, want %q", got, want)
	}
}

func TestEnsureAccessKeyGeneratesWhenMissing(t *testing.T) {
	key, generated, err := ensureAccessKey("")
	if err != nil {
		t.Fatalf("ensureAccessKey error: %v", err)
	}
	if !generated {
		t.Fatal("expected generated key")
	}
	if len(key) != 32 {
		t.Fatalf("generated key length = %d, want 32", len(key))
	}
}

func TestBuildPanelURLRewritesWildcardHost(t *testing.T) {
	got := buildPanelURL("0.0.0.0", 8443, false, "secret")
	want := "http://localhost:8443/?token=secret"
	if got != want {
		t.Fatalf("buildPanelURL = %q, want %q", got, want)
	}
}

func TestPrintPanelAccessGeneratedKey(t *testing.T) {
	var outBuf bytes.Buffer

	prevOut := stdout
	stdout = &outBuf
	t.Cleanup(func() {
		stdout = prevOut
	})

	got := printPanelAccess("0.0.0.0", 8443, true, "secret", true)
	wantURL := "https://localhost:8443/?token=secret"
	if got != wantURL {
		t.Fatalf("printPanelAccess URL = %q, want %q", got, wantURL)
	}

	out := outBuf.String()
	for _, want := range []string{
		"Listening on: 0.0.0.0:8443",
		"Panel URL: https://localhost:8443/?token=secret",
		"API key: secret (generated)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("printPanelAccess output %q missing %q", out, want)
		}
	}
}
