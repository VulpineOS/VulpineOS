//go:build !race

package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"vulpineos/internal/config"
	"vulpineos/internal/openclaw"
	"vulpineos/internal/orchestrator"
	"vulpineos/internal/vault"
)

func TestPanelAgentSessionSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("skip soak in short mode")
	}
	if strings.TrimSpace(os.Getenv("VULPINEOS_RUN_SOAK")) == "" {
		t.Skip("set VULPINEOS_RUN_SOAK=1 to run the panel-agent soak")
	}

	mgr := openclaw.NewManager("")
	if !mgr.OpenClawInstalled() {
		t.Skip("OpenClaw not installed")
	}
	defer mgr.Dispose()

	cfg, err := config.Load()
	if err != nil || !cfg.SetupComplete {
		t.Skip("VulpineOS not configured — run setup wizard first")
	}
	if err := cfg.GenerateOpenClawConfig("", cfg.BinaryPath); err != nil {
		t.Fatalf("GenerateOpenClawConfig: %v", err)
	}

	tmp := t.TempDir()
	v, err := vault.OpenPath(tmp + "/vault.db")
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer v.Close()

	orch := &orchestrator.Orchestrator{Agents: mgr}
	server := NewServer(":0", "secret", nil)
	server.SetPanelAPI(&PanelAPI{
		Orchestrator: orch,
		Vault:        v,
	})

	statusCh := mgr.StatusChan()
	go func() {
		for status := range statusCh {
			_ = v.UpdateAgentStatus(status.AgentID, status.Status)
			if status.Tokens > 0 {
				_ = v.UpdateAgentTokens(status.AgentID, status.Tokens)
			}
			payload, _ := json.Marshal(map[string]interface{}{
				"agentId": status.AgentID,
				"status":  status.Status,
				"tokens":  status.Tokens,
			})
			server.BroadcastEvent("Vulpine.agentStatus", "", payload)
		}
	}()

	conversationCh := mgr.ConversationChan()
	go func() {
		for msg := range conversationCh {
			_ = v.AppendMessage(msg.AgentID, msg.Role, msg.Content, msg.Tokens)
			payload, _ := json.Marshal(map[string]interface{}{
				"agentId": msg.AgentID,
				"role":    msg.Role,
				"content": msg.Content,
				"tokens":  msg.Tokens,
			})
			server.BroadcastEvent("Vulpine.conversation", "", payload)
		}
	}()

	httpServer := httptest.NewServer(server.Mux())
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws?token=secret"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	type agentPair struct {
		id    string
		token string
	}
	agents := []agentPair{
		{id: "", token: "PANEL-ALPHA"},
		{id: "", token: "PANEL-BETA"},
	}
	spawnEvents := make([]map[string]interface{}, 0)

	for i := range agents {
		resp, events := controlCall(t, ctx, conn, "agents.spawn", map[string]interface{}{
			"task": fmt.Sprintf("Remember token %s and reply exactly SAVED:%s", agents[i].token, agents[i].token),
		})
		spawnEvents = append(spawnEvents, events...)
		agents[i].id = resultString(t, resp, "agentId")
	}

	waitForConversationEvents(t, ctx, conn, spawnEvents, map[string]string{
		agents[0].id: "SAVED:" + agents[0].token,
		agents[1].id: "SAVED:" + agents[1].token,
	})

	tokenEvents := make([]map[string]interface{}, 0)
	for _, agent := range agents {
		resp, events := controlCall(t, ctx, conn, "agents.resume", map[string]interface{}{
			"agentId": agent.id,
			"message": fmt.Sprintf("What token did I ask you to remember? Reply exactly TOKEN:%s", agent.token),
		})
		tokenEvents = append(tokenEvents, events...)
		if resultString(t, resp, "agentId") != agent.id {
			t.Fatalf("resume agentId = %#v", resp)
		}
	}

	waitForConversationEvents(t, ctx, conn, tokenEvents, map[string]string{
		agents[0].id: "TOKEN:" + agents[0].token,
		agents[1].id: "TOKEN:" + agents[1].token,
	})

	for _, agent := range agents {
		resp, _ := controlCall(t, ctx, conn, "agents.getMessages", map[string]interface{}{
			"agentId": agent.id,
		})
		messages, ok := resp["result"].(map[string]interface{})["messages"].([]interface{})
		if !ok {
			t.Fatalf("messages = %#v", resp)
		}
		if len(messages) < 4 {
			t.Fatalf("messages len = %d", len(messages))
		}
	}

	pauseToken := "PAUSE-ALPHA"
	if err := startAndInterruptTurn(t, ctx, conn, "agents.resume", "agents.pause", agents[0].id, longOutputPrompt(pauseToken)); err != nil {
		t.Fatalf("pause agent: %v", err)
	}
	waitForAgentStatusInVault(t, v, agents[0].id, "paused", 30*time.Second)

	killToken := "KILL-BETA"
	if err := startAndInterruptTurn(t, ctx, conn, "agents.resume", "agents.kill", agents[1].id, longOutputPrompt(killToken)); err != nil {
		t.Fatalf("kill agent: %v", err)
	}
	waitForAgentStatusInVault(t, v, agents[1].id, "completed", 30*time.Second)

	resp, _ := controlCall(t, ctx, conn, "agents.getMessages", map[string]interface{}{
		"agentId": agents[0].id,
	})
	assertMessagesContain(t, resp, "TOKEN:"+agents[0].token, pauseToken)

	resp, _ = controlCall(t, ctx, conn, "agents.getMessages", map[string]interface{}{
		"agentId": agents[1].id,
	})
	assertMessagesContain(t, resp, "SAVED:"+agents[1].token, "TOKEN:"+agents[1].token, killToken)
}

func controlCall(t *testing.T, ctx context.Context, conn *websocket.Conn, method string, params map[string]interface{}) (map[string]interface{}, []map[string]interface{}) {
	t.Helper()
	id := time.Now().Nanosecond()
	payload, err := json.Marshal(map[string]interface{}{
		"type": "control",
		"payload": map[string]interface{}{
			"command": method,
			"params":  params,
			"id":      id,
		},
	})
	if err != nil {
		t.Fatalf("marshal control payload: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatalf("write control payload: %v", err)
	}

	events := make([]map[string]interface{}, 0)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read websocket: %v", err)
		}
		var env map[string]interface{}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		switch env["type"] {
		case "control":
			payload, ok := env["payload"].(map[string]interface{})
			if !ok {
				t.Fatalf("control payload = %#v", env)
			}
			params, ok := payload["params"].(map[string]interface{})
			if !ok {
				t.Fatalf("control params = %#v", payload)
			}
			if int(params["id"].(float64)) == id {
				return params, events
			}
		case "juggler":
			var event map[string]interface{}
			if err := json.Unmarshal(anyToBytes(t, env["payload"]), &event); err != nil {
				t.Fatalf("unmarshal juggler payload: %v", err)
			}
			events = append(events, event)
		}
	}
}

func waitForConversationEvents(t *testing.T, ctx context.Context, conn *websocket.Conn, initial []map[string]interface{}, wants map[string]string) {
	t.Helper()
	pending := make(map[string]string, len(wants))
	for agentID, want := range wants {
		pending[agentID] = want
	}
	for _, payload := range initial {
		markConversationMatch(pending, payload)
	}

	deadline := time.Now().Add(2 * time.Minute)
	for len(pending) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("pending conversation events: %#v", pending)
		}
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read websocket: %v", err)
		}
		var env map[string]interface{}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env["type"] != "juggler" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(anyToBytes(t, env["payload"]), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload["method"] != "Vulpine.conversation" {
			continue
		}
		markConversationMatch(pending, payload)
	}
}

func markConversationMatch(pending map[string]string, payload map[string]interface{}) {
	params, ok := payload["params"].(map[string]interface{})
	if !ok {
		return
	}
	agentID, _ := params["agentId"].(string)
	content, _ := params["content"].(string)
	role, _ := params["role"].(string)
	want, exists := pending[agentID]
	if exists && role == "assistant" && strings.Contains(content, want) {
		delete(pending, agentID)
	}
}

func anyToBytes(t *testing.T, v interface{}) []byte {
	t.Helper()
	switch x := v.(type) {
	case string:
		return []byte(x)
	default:
		data, err := json.Marshal(x)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		return data
	}
}

func resultString(t *testing.T, resp map[string]interface{}, key string) string {
	t.Helper()
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result = %#v", resp)
	}
	value, _ := result[key].(string)
	return value
}

func startAndInterruptTurn(t *testing.T, ctx context.Context, conn *websocket.Conn, startMethod, interruptMethod, agentID, message string) error {
	t.Helper()

	for attempt := 0; attempt < 5; attempt++ {
		startResp, _ := controlCall(t, ctx, conn, startMethod, map[string]interface{}{
			"agentId": agentID,
			"message": message,
		})
		if errText := responseError(startResp); errText != "" {
			return fmt.Errorf("start turn: %s", errText)
		}

		interruptResp, _ := controlCall(t, ctx, conn, interruptMethod, map[string]interface{}{
			"agentId": agentID,
		})
		if errText := responseError(interruptResp); errText == "" {
			return nil
		} else if !strings.Contains(errText, "not found") {
			return fmt.Errorf("%s: %s", interruptMethod, errText)
		}

		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("%s never caught a live turn", interruptMethod)
}

func waitForAgentStatusInVault(t *testing.T, v *vault.DB, agentID, want string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		agent, err := v.GetAgent(agentID)
		if err != nil {
			t.Fatalf("get agent %s: %v", agentID, err)
		}
		if agent.Status == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	agent, err := v.GetAgent(agentID)
	if err != nil {
		t.Fatalf("get agent %s after timeout: %v", agentID, err)
	}
	t.Fatalf("agent %s status = %q, want %q", agentID, agent.Status, want)
}

func assertMessagesContain(t *testing.T, resp map[string]interface{}, wants ...string) {
	t.Helper()

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result = %#v", resp)
	}
	messages, ok := result["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages = %#v", resp)
	}

	joined := make([]string, 0, len(messages))
	for _, raw := range messages {
		msg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		content, _ := msg["content"].(string)
		joined = append(joined, content)
	}
	haystack := strings.Join(joined, "\n")
	for _, want := range wants {
		if !strings.Contains(haystack, want) {
			t.Fatalf("messages missing %q in:\n%s", want, haystack)
		}
	}
}

func responseError(resp map[string]interface{}) string {
	if resp == nil {
		return "nil response"
	}
	errText, _ := resp["error"].(string)
	return errText
}

func longOutputPrompt(token string) string {
	return fmt.Sprintf("Return exactly 600 lines. Each line must begin with HOLD:%s: followed by a zero-padded line number from 001 to 600. Do not summarize, explain, or stop early.", token)
}
