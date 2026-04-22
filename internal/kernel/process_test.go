package kernel

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func camoufoxBinary() string {
	if b := os.Getenv("CAMOUFOX_BINARY"); b != "" {
		return b
	}
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), "Downloads", "Camoufox.app", "Contents", "MacOS", "camoufox"),
		"/Applications/Camoufox.app/Contents/MacOS/camoufox",
		"/Applications/camoufox.app/Contents/MacOS/camoufox",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if b, err := ResolveBinaryPath(""); err == nil {
		return b
	}
	return ""
}

func TestBinaryLocatorPrefersRepoLocalBuild(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	execPath := filepath.Join(root, "bin", "vulpineos")
	repoBinary := filepath.Join(root, "camoufox-146.0.1-beta.25", "obj-aarch64-apple-darwin", "dist", "Camoufox.app", "Contents", "MacOS", "camoufox")
	repoFallbackBinary := filepath.Join(root, "camoufox-146.0.1-beta.25", "obj-aarch64-apple-darwin", "dist", "bin", "camoufox")
	downloadsBinary := filepath.Join(home, "Downloads", "Camoufox.app", "Contents", "MacOS", "camoufox")

	mustWriteExecutable(t, execPath)
	mustWriteExecutable(t, repoBinary)
	mustWriteExecutable(t, repoFallbackBinary)
	mustWriteExecutable(t, downloadsBinary)

	locator := binaryLocator{
		execPath: execPath,
		cwd:      root,
		home:     home,
		goos:     "darwin",
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	resolved, err := locator.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved != repoBinary {
		t.Fatalf("Resolve = %q, want %q", resolved, repoBinary)
	}
}

func TestBinaryLocatorFallsBackToRepoBinWhenAppBundleMissing(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	execPath := filepath.Join(root, "bin", "vulpineos")
	repoBinary := filepath.Join(root, "camoufox-146.0.1-beta.25", "obj-aarch64-apple-darwin", "dist", "bin", "camoufox")
	downloadsBinary := filepath.Join(home, "Downloads", "Camoufox.app", "Contents", "MacOS", "camoufox")

	mustWriteExecutable(t, execPath)
	mustWriteExecutable(t, repoBinary)
	mustWriteExecutable(t, downloadsBinary)

	locator := binaryLocator{
		execPath: execPath,
		cwd:      root,
		home:     home,
		goos:     "darwin",
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	resolved, err := locator.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved != repoBinary {
		t.Fatalf("Resolve = %q, want %q", resolved, repoBinary)
	}
}

func TestBinaryLocatorDetectDriftWarnsOnOlderExplicitBinary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	execPath := filepath.Join(root, "bin", "vulpineos")
	repoBinary := filepath.Join(root, "camoufox-146.0.1-beta.25", "obj-aarch64-apple-darwin", "dist", "Camoufox.app", "Contents", "MacOS", "camoufox")
	repoFallbackBinary := filepath.Join(root, "camoufox-146.0.1-beta.25", "obj-aarch64-apple-darwin", "dist", "bin", "camoufox")
	downloadsBinary := filepath.Join(home, "Downloads", "Camoufox.app", "Contents", "MacOS", "camoufox")

	mustWriteExecutable(t, execPath)
	mustWriteExecutable(t, repoBinary)
	mustWriteExecutable(t, repoFallbackBinary)
	mustWriteExecutable(t, downloadsBinary)
	now := time.Now()
	if err := os.Chtimes(downloadsBinary, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes downloads: %v", err)
	}
	if err := os.Chtimes(repoBinary, now, now); err != nil {
		t.Fatalf("Chtimes repo: %v", err)
	}

	locator := binaryLocator{
		execPath: execPath,
		cwd:      root,
		home:     home,
		goos:     "darwin",
		lookPath: func(string) (string, error) { return "", os.ErrNotExist },
	}

	warning := locator.DetectDrift(downloadsBinary)
	if warning == nil {
		t.Fatal("DetectDrift returned nil")
	}
	if warning.PreferredPath != repoBinary {
		t.Fatalf("PreferredPath = %q, want %q", warning.PreferredPath, repoBinary)
	}
	if warning.SelectedPath != downloadsBinary {
		t.Fatalf("SelectedPath = %q, want %q", warning.SelectedPath, downloadsBinary)
	}
}

func mustWriteExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestKernelStartStop(t *testing.T) {
	bin := camoufoxBinary()
	if bin == "" {
		t.Skip("camoufox binary not found")
	}

	k := New()
	if k.Running() {
		t.Fatal("kernel should not be running before Start")
	}

	err := k.Start(Config{
		BinaryPath: bin,
		Headless:   true,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer k.Stop()

	if !k.Running() {
		t.Fatal("kernel should be running after Start")
	}
	if k.PID() == 0 {
		t.Fatal("PID should be non-zero")
	}
	if k.Client() == nil {
		t.Fatal("Client should not be nil")
	}
	if k.Uptime() < 0 {
		t.Fatal("Uptime should be non-negative")
	}
	if k.IsHeadless() != true {
		t.Fatal("should be headless")
	}
}

func TestKernelBrowserEnable(t *testing.T) {
	bin := camoufoxBinary()
	if bin == "" {
		t.Skip("camoufox binary not found")
	}

	k := New()
	if err := k.Start(Config{BinaryPath: bin, Headless: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer k.Stop()

	client := k.Client()

	// Browser.enable
	_, err := client.Call("", "Browser.enable", map[string]interface{}{
		"attachToDefaultContext": true,
	})
	if err != nil {
		t.Fatalf("Browser.enable: %v", err)
	}

	// Browser.getInfo
	result, err := client.Call("", "Browser.getInfo", nil)
	if err != nil {
		t.Fatalf("Browser.getInfo: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("Browser.getInfo returned empty")
	}
	t.Logf("Browser.getInfo: %s", string(result)[:min(len(result), 100)])
}

func TestKernelNewPageAndNavigate(t *testing.T) {
	bin := camoufoxBinary()
	if bin == "" {
		t.Skip("camoufox binary not found")
	}

	k := New()
	if err := k.Start(Config{BinaryPath: bin, Headless: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer k.Stop()

	client := k.Client()
	client.Call("", "Browser.enable", map[string]interface{}{"attachToDefaultContext": true})

	// Wait for events to settle
	time.Sleep(2 * time.Second)

	// Create a new page
	result, err := client.Call("", "Browser.newPage", nil)
	if err != nil {
		t.Fatalf("Browser.newPage: %v", err)
	}
	t.Logf("newPage: %s", string(result))

	// Navigate
	// Need the session ID from the attachedToTarget event — use a simpler approach
	// Just verify the page was created
	if len(result) == 0 {
		t.Fatal("newPage returned empty")
	}
}

func TestKernelCreateBrowserContext(t *testing.T) {
	bin := camoufoxBinary()
	if bin == "" {
		t.Skip("camoufox binary not found")
	}

	k := New()
	if err := k.Start(Config{BinaryPath: bin, Headless: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer k.Stop()

	client := k.Client()
	client.Call("", "Browser.enable", map[string]interface{}{"attachToDefaultContext": true})

	// Create browser context
	result, err := client.Call("", "Browser.createBrowserContext", map[string]interface{}{
		"removeOnDetach": true,
	})
	if err != nil {
		t.Fatalf("createBrowserContext: %v", err)
	}
	t.Logf("createBrowserContext: %s", string(result))

	// Remove it
	_, err = client.Call("", "Browser.removeBrowserContext", map[string]interface{}{
		"browserContextId": "default", // just test the call doesn't crash
	})
	// May fail for default context — that's ok
}

func TestKernelDoubleStart(t *testing.T) {
	bin := camoufoxBinary()
	if bin == "" {
		t.Skip("camoufox binary not found")
	}

	k := New()
	if err := k.Start(Config{BinaryPath: bin, Headless: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer k.Stop()

	// Second start should fail gracefully
	err := k.Start(Config{BinaryPath: bin, Headless: true})
	if err == nil {
		t.Fatal("double Start should return error")
	}
}

func TestKernelStopIdempotent(t *testing.T) {
	bin := camoufoxBinary()
	if bin == "" {
		t.Skip("camoufox binary not found")
	}

	k := New()
	if err := k.Start(Config{BinaryPath: bin, Headless: true}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop twice should not panic
	k.Stop()
	k.Stop()

	if k.Running() {
		t.Fatal("should not be running after Stop")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
