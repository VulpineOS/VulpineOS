package recording

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestRecordAndGetTimeline(t *testing.T) {
	r := NewRecorder()

	data1 := json.RawMessage(`{"url":"https://example.com"}`)
	data2 := json.RawMessage(`{"x":100,"y":200}`)

	r.Record("agent-1", ActionNavigate, data1)
	time.Sleep(time.Millisecond) // ensure distinct timestamps
	r.Record("agent-1", ActionClick, data2)

	timeline := r.GetTimeline("agent-1")
	if len(timeline) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(timeline))
	}
	if timeline[0].Type != ActionNavigate {
		t.Errorf("expected first action navigate, got %s", timeline[0].Type)
	}
	if timeline[1].Type != ActionClick {
		t.Errorf("expected second action click, got %s", timeline[1].Type)
	}
	if !timeline[0].Timestamp.Before(timeline[1].Timestamp) {
		t.Error("expected actions sorted by timestamp")
	}
}

func TestExportReturnsValidJSON(t *testing.T) {
	r := NewRecorder()

	r.Record("agent-1", ActionNavigate, json.RawMessage(`{"url":"https://example.com"}`))
	r.Record("agent-1", ActionClick, json.RawMessage(`{"x":50,"y":75}`))

	data, err := r.Export("agent-1")
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	var actions []Action
	if err := json.Unmarshal(data, &actions); err != nil {
		t.Fatalf("Export returned invalid JSON: %v", err)
	}
	if len(actions) != 2 {
		t.Errorf("expected 2 actions in export, got %d", len(actions))
	}
}

func TestRecorderTrimsOldestActionsAtLimit(t *testing.T) {
	r := NewRecorderWithLimit(3)

	for i := 0; i < 5; i++ {
		r.Record("agent-1", ActionNavigate, json.RawMessage(fmt.Sprintf(`{"url":"https://example.com/%d"}`, i)))
	}

	timeline := r.GetTimeline("agent-1")
	if len(timeline) != 3 {
		t.Fatalf("expected 3 retained actions, got %d", len(timeline))
	}
	for i, action := range timeline {
		var data map[string]string
		if err := json.Unmarshal(action.Data, &data); err != nil {
			t.Fatalf("unmarshal action %d: %v", i, err)
		}
		want := fmt.Sprintf("https://example.com/%d", i+2)
		if data["url"] != want {
			t.Fatalf("action %d url = %q, want %q", i, data["url"], want)
		}
	}
}

func TestRecorderRedactsSensitiveActionData(t *testing.T) {
	r := NewRecorder()
	r.Record("agent-1", ActionType_, json.RawMessage(`{
		"selector":"input[type=password]",
		"text":"correct-horse",
		"headers":{"Authorization":"Bearer token-123"},
		"apiKey":"key-123",
		"nested":{"cookie":"session=abc"}
	}`))
	r.Record("agent-1", ActionType_, json.RawMessage(`{"selector":"input[name=q]","text":"public search"}`))

	timeline := r.GetTimeline("agent-1")
	if len(timeline) != 2 {
		t.Fatalf("expected 2 retained actions, got %d", len(timeline))
	}

	var secret map[string]interface{}
	if err := json.Unmarshal(timeline[0].Data, &secret); err != nil {
		t.Fatalf("unmarshal redacted action: %v", err)
	}
	if secret["text"] != "[redacted]" || secret["apiKey"] != "[redacted]" {
		t.Fatalf("sensitive top-level values were not redacted: %#v", secret)
	}
	headers, ok := secret["headers"].(map[string]interface{})
	if !ok || headers["Authorization"] != "[redacted]" {
		t.Fatalf("authorization header was not redacted: %#v", secret["headers"])
	}
	nested, ok := secret["nested"].(map[string]interface{})
	if !ok || nested["cookie"] != "[redacted]" {
		t.Fatalf("nested cookie was not redacted: %#v", secret["nested"])
	}

	var public map[string]interface{}
	if err := json.Unmarshal(timeline[1].Data, &public); err != nil {
		t.Fatalf("unmarshal public action: %v", err)
	}
	if public["text"] != "public search" {
		t.Fatalf("public text should be preserved: %#v", public)
	}
}

func TestExportEmptyTimeline(t *testing.T) {
	r := NewRecorder()

	data, err := r.Export("nonexistent")
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	var actions []Action
	if err := json.Unmarshal(data, &actions); err != nil {
		t.Fatalf("Export returned invalid JSON: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected empty array, got %d actions", len(actions))
	}
}

func TestClearRemovesActions(t *testing.T) {
	r := NewRecorder()

	r.Record("agent-1", ActionNavigate, json.RawMessage(`{"url":"https://example.com"}`))
	r.Record("agent-1", ActionClick, json.RawMessage(`{"x":10,"y":20}`))

	r.Clear("agent-1")

	timeline := r.GetTimeline("agent-1")
	if len(timeline) != 0 {
		t.Errorf("expected empty timeline after clear, got %d actions", len(timeline))
	}
}

func TestEmptyTimeline(t *testing.T) {
	r := NewRecorder()

	timeline := r.GetTimeline("agent-1")
	if timeline != nil {
		t.Errorf("expected nil timeline for unknown agent, got %v", timeline)
	}
}

func TestMultipleAgentsDontInterfere(t *testing.T) {
	r := NewRecorder()

	r.Record("agent-1", ActionNavigate, json.RawMessage(`{"url":"https://a.com"}`))
	r.Record("agent-2", ActionClick, json.RawMessage(`{"x":1,"y":2}`))
	r.Record("agent-1", ActionScroll, json.RawMessage(`{"deltaY":100}`))

	t1 := r.GetTimeline("agent-1")
	t2 := r.GetTimeline("agent-2")

	if len(t1) != 2 {
		t.Errorf("agent-1: expected 2 actions, got %d", len(t1))
	}
	if len(t2) != 1 {
		t.Errorf("agent-2: expected 1 action, got %d", len(t2))
	}

	// Verify agent-1 actions are correct types
	if t1[0].Type != ActionNavigate || t1[1].Type != ActionScroll {
		t.Errorf("agent-1: unexpected action types: %s, %s", t1[0].Type, t1[1].Type)
	}
	if t2[0].Type != ActionClick {
		t.Errorf("agent-2: expected click, got %s", t2[0].Type)
	}

	// Clear agent-1 should not affect agent-2
	r.Clear("agent-1")
	if len(r.GetTimeline("agent-2")) != 1 {
		t.Error("clearing agent-1 affected agent-2")
	}
}
