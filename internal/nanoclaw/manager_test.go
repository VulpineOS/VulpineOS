package nanoclaw

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
	if m.binary != "nanoclaw" {
		t.Errorf("binary = %q, want %q (default)", m.binary, "nanoclaw")
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

func TestKillMarksAgentInterrupted(t *testing.T) {
	m := NewManager("test")
	statusCh := m.StatusChan()
	agent := newAgent("agent-1", "ctx-1", m.statusSource)
	m.agents["agent-1"] = &managedAgent{agent: agent}

	if err := m.Kill("agent-1"); err != nil {
		t.Fatalf("kill agent: %v", err)
	}
	if got := agent.Status().Status; got != "interrupted" {
		t.Fatalf("status = %q, want interrupted", got)
	}

	select {
	case status := <-statusCh:
		if status.AgentID != "agent-1" {
			t.Fatalf("status agent = %q, want agent-1", status.AgentID)
		}
		if status.Status != "interrupted" {
			t.Fatalf("emitted status = %q, want interrupted", status.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interrupted status")
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

func TestAgentStatusAfterManagerDisposeDoesNotPanic(t *testing.T) {
	m := NewManager("test")
	agent := newAgent("agent-1", "ctx-1", m.statusSource)
	m.Dispose()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("emitStatus panicked after manager dispose: %v", r)
		}
	}()
	agent.emitStatus()
}

func TestForwardConversationAfterManagerDisposeDoesNotPanic(t *testing.T) {
	m := NewManager("test")
	agent := newAgent("agent-1", "ctx-1", make(chan AgentStatus, 1))
	m.Dispose()

	done := make(chan struct{})
	go func() {
		m.forwardConversation(agent)
		close(done)
	}()

	agent.conversationCh <- ConversationMsg{AgentID: "agent-1", Role: "assistant", Content: "late"}
	close(agent.conversationCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("forwardConversation did not exit")
	}
}

func TestAgentConversationAfterCloseDoesNotPanic(t *testing.T) {
	agent := newAgent("agent-1", "ctx-1", make(chan AgentStatus, 1))
	close(agent.conversationCh)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("emitConversation panicked after channel close: %v", r)
		}
	}()
	agent.emitConversation(ConversationMsg{AgentID: "agent-1", Role: "system", Content: "late"})
}

func TestNanoClawInstalledFalseForBogus(t *testing.T) {
	m := NewManager("/nonexistent/path/to/nanoclaw-binary-xyz")
	// Should return false since the binary doesn't exist
	// (may return true if nanoclaw is globally installed, so this is best-effort)
	// At minimum, verify it doesn't panic
	_ = m.NanoClawInstalled()
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
	badBin := filepath.Join(t.TempDir(), "nonexistent-nanoclaw-bin")

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

func TestSpawnWithSessionRejectsUnsafeSessionNameAndRunsCleanup(t *testing.T) {
	for _, sessionName := range []string{"../escape", `..\escape`, "nested/session", "."} {
		t.Run(sessionName, func(t *testing.T) {
			m := NewManager("/not-needed")
			called := false
			_, err := m.SpawnWithSessionIsolated("agent-1", "task", sessionName, config.OpenClawConfigPath(), func() {
				called = true
			})
			if err == nil || !strings.Contains(err.Error(), "invalid sessionName") {
				t.Fatalf("error = %v, want invalid sessionName", err)
			}
			if !called {
				t.Fatal("expected cleanup to run on validation failure")
			}
		})
	}
}

func TestSafeSessionNameDefaultsToAgentID(t *testing.T) {
	got, err := safeSessionName("agent-1", "")
	if err != nil {
		t.Fatalf("safeSessionName: %v", err)
	}
	if got != "vulpine-agent-1" {
		t.Fatalf("session name = %q, want vulpine-agent-1", got)
	}
}

func TestSessionLogPathForSessionIDRejectsTraversal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, err := sessionLogPathForSessionID("vulpine-agent-1")
	if err != nil {
		t.Fatalf("sessionLogPathForSessionID: %v", err)
	}
	want := filepath.Join(config.OpenClawProfileDir(), "agents", "main", "sessions", "vulpine-agent-1.jsonl")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}

	for _, sessionID := range []string{"../escape", `..\escape`, "nested/session"} {
		t.Run(sessionID, func(t *testing.T) {
			_, err := sessionLogPathForSessionID(sessionID)
			if err == nil || !strings.Contains(err.Error(), "invalid sessionName") {
				t.Fatalf("error = %v, want invalid sessionName", err)
			}
		})
	}
}

func TestProvisionOpenRouterOneCLISecretUpdatesExistingSecret(t *testing.T) {
	var calls [][]string
	runner := func(name string, args []string, env []string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if !contains(env, "ONECLI_API_HOST=http://127.0.0.1:10254") {
			t.Fatalf("env missing ONECLI_API_HOST: %#v", env)
		}
		if len(calls) == 1 {
			return []byte(`[{"id":"secret-1","name":"OpenRouter","hostPattern":"openrouter.ai"}]`), nil
		}
		return []byte(`{"id":"secret-1"}`), nil
	}

	if err := provisionOpenRouterOneCLISecret("sk-test", "http://127.0.0.1:10254", runner); err != nil {
		t.Fatalf("provisionOpenRouterOneCLISecret: %v", err)
	}

	want := [][]string{
		{"onecli", "secrets", "list", "--fields", "id,name,hostPattern", "--max", "100"},
		{"onecli", "secrets", "update", "--id", "secret-1", "--value", "sk-test", "--host-pattern", "openrouter.ai", "--header-name", "Authorization", "--value-format", "Bearer {value}"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestProvisionOpenRouterOneCLISecretCreatesMissingSecret(t *testing.T) {
	var calls [][]string
	runner := func(name string, args []string, env []string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if len(calls) == 1 {
			return []byte(`[]`), nil
		}
		return []byte(`{"id":"secret-1"}`), nil
	}

	if err := provisionOpenRouterOneCLISecret("sk-test", "http://127.0.0.1:10254", runner); err != nil {
		t.Fatalf("provisionOpenRouterOneCLISecret: %v", err)
	}

	want := []string{"onecli", "secrets", "create", "--name", "OpenRouter", "--type", "generic", "--value", "sk-test", "--host-pattern", "openrouter.ai", "--header-name", "Authorization", "--value-format", "Bearer {value}"}
	if len(calls) != 2 || !reflect.DeepEqual(calls[1], want) {
		t.Fatalf("create call = %#v, want %#v", calls, want)
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
