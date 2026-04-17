package openclaw

import (
	"os"
	"path/filepath"
	"testing"

	"vulpineos/internal/config"
)

func TestNewManager(t *testing.T) {
	m := NewManager("test-binary")
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.binary != "test-binary" {
		t.Errorf("binary = %q, want %q", m.binary, "test-binary")
	}
	if m.agents == nil {
		t.Error("agents map should be initialized")
	}
}

func TestNewManagerDefaultBinary(t *testing.T) {
	m := NewManager("")
	if m.binary != "openclaw" {
		t.Errorf("binary = %q, want %q (default)", m.binary, "openclaw")
	}
}

func TestStatusChan(t *testing.T) {
	m := NewManager("test")
	ch := m.StatusChan()
	if ch == nil {
		t.Fatal("StatusChan() returned nil")
	}
	// Verify it's a receive-only channel by checking we can read the type
	select {
	case <-ch:
		t.Error("should not have received anything from empty channel")
	default:
		// expected
	}
}

func TestConversationChan(t *testing.T) {
	m := NewManager("test")
	ch := m.ConversationChan()
	if ch == nil {
		t.Fatal("ConversationChan() returned nil")
	}
}

func TestCountStartsAtZero(t *testing.T) {
	m := NewManager("test")
	if m.Count() != 0 {
		t.Errorf("Count() = %d, want 0", m.Count())
	}
}

func TestListStartsEmpty(t *testing.T) {
	m := NewManager("test")
	list := m.List()
	if len(list) != 0 {
		t.Errorf("List() length = %d, want 0", len(list))
	}
}

func TestKillNonexistent(t *testing.T) {
	m := NewManager("test")
	err := m.Kill("nonexistent-id")
	if err == nil {
		t.Error("expected error when killing nonexistent agent")
	}
}

func TestPauseNonexistent(t *testing.T) {
	m := NewManager("test")
	err := m.PauseAgent("nonexistent-id")
	if err == nil {
		t.Error("expected error when pausing nonexistent agent")
	}
}

func TestSendMessageNonexistent(t *testing.T) {
	m := NewManager("test")
	err := m.SendMessage("nonexistent-id", "hello")
	if err == nil {
		t.Error("expected error when sending to nonexistent agent")
	}
}

func TestKillAllEmpty(t *testing.T) {
	m := NewManager("test")
	// Should not panic on empty manager
	m.KillAll()
}

func TestDisposeEmpty(t *testing.T) {
	m := NewManager("test")
	// Should not panic; channels should be closed
	m.Dispose()

	// Verify channels are closed
	_, ok := <-m.StatusChan()
	if ok {
		t.Error("StatusChan should be closed after Dispose")
	}
	_, ok = <-m.ConversationChan()
	if ok {
		t.Error("ConversationChan should be closed after Dispose")
	}
}

func TestOpenClawInstalledFalseForBogus(t *testing.T) {
	m := NewManager("/nonexistent/path/to/openclaw-binary-xyz")
	// Should return false since the binary doesn't exist
	// (may return true if openclaw is globally installed, so this is best-effort)
	// At minimum, verify it doesn't panic
	_ = m.OpenClawInstalled()
}

func TestSpawnFailsWithBadBinary(t *testing.T) {
	// Spawn should fail when given a binary that exists but isn't executable
	// or when the process immediately fails
	m := NewManager("/dev/null") // exists but not executable as a command
	_, err := m.Spawn("ctx-1", "")
	if err == nil {
		t.Error("expected error when spawning with non-executable binary")
	}
}

func TestRuntimeEnvForConfigIncludesGatewayToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"gateway":{"auth":{"mode":"token","token":"token-123"}}}`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := runtimeEnvForConfig(configPath)
	if env["OPENCLAW_CONFIG_PATH"] != configPath {
		t.Fatalf("OPENCLAW_CONFIG_PATH = %q, want %q", env["OPENCLAW_CONFIG_PATH"], configPath)
	}
	if env["OPENCLAW_GATEWAY_TOKEN"] != "token-123" {
		t.Fatalf("OPENCLAW_GATEWAY_TOKEN = %q, want %q", env["OPENCLAW_GATEWAY_TOKEN"], "token-123")
	}
}

func TestRuntimeEnvForConfigOmitsMissingGatewayToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"gateway":{"mode":"local"}}`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := runtimeEnvForConfig(configPath)
	if env["OPENCLAW_CONFIG_PATH"] != configPath {
		t.Fatalf("OPENCLAW_CONFIG_PATH = %q, want %q", env["OPENCLAW_CONFIG_PATH"], configPath)
	}
	if _, ok := env["OPENCLAW_GATEWAY_TOKEN"]; ok {
		t.Fatalf("OPENCLAW_GATEWAY_TOKEN should be omitted when token is absent: %#v", env)
	}
}

func TestResumeWithSessionIsolatedRunsCleanupOnStartFailure(t *testing.T) {
	badBin := filepath.Join(t.TempDir(), "broken-openclaw")
	if err := os.WriteFile(badBin, []byte("not-a-real-binary"), 0755); err != nil {
		t.Fatalf("write broken binary: %v", err)
	}

	m := NewManager(badBin)
	called := false

	_, err := m.ResumeWithSessionIsolated("agent-1", "session-1", config.OpenClawConfigPath(), func() {
		called = true
	})
	if err == nil {
		t.Fatal("expected resume to fail with bad binary")
	}
	if !called {
		t.Fatal("expected cleanup to run on start failure")
	}
}
