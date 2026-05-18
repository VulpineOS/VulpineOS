package main

import (
	"bytes"
	"strings"
	"testing"
)

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

func TestRun_HelpFlagsExitZero(t *testing.T) {
	for _, args := range [][]string{
		{"vulpineos", "--help"},
		{"vulpineos", "tui", "--help"},
		{"vulpineos", "mcp", "--help"},
	} {
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			var outBuf, errBuf bytes.Buffer

			prevOut, prevErr := stdout, stderr
			stdout = &outBuf
			stderr = &errBuf
			t.Cleanup(func() {
				stdout = prevOut
				stderr = prevErr
			})

			if code := Run(args); code != 0 {
				t.Fatalf("Run(%v) exit code = %d, want 0 (stderr=%q)", args, code, errBuf.String())
			}
			if strings.TrimSpace(errBuf.String()) == "" {
				t.Fatalf("Run(%v) should print help text to stderr", args)
			}
		})
	}
}
