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
