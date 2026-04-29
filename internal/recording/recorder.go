package recording

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"vulpineos/internal/juggler"
)

// ActionType identifies what kind of browser action was recorded.
type ActionType string

const (
	ActionNavigate   ActionType = "navigate"
	ActionClick      ActionType = "click"
	ActionType_      ActionType = "type"
	ActionScroll     ActionType = "scroll"
	ActionScreenshot ActionType = "screenshot"
	ActionEvaluate   ActionType = "evaluate"
	ActionSetContent ActionType = "setContent"
)

// Action is a single recorded browser action with timestamp.
type Action struct {
	AgentID   string          `json:"agentId"`
	Type      ActionType      `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// Recorder stores browser actions per agent as replayable timelines.
// Recordings are ephemeral (in-memory only, not persisted to vault).
type Recorder struct {
	mu                 sync.Mutex
	maxActionsPerAgent int
	actions            map[string][]Action // agentID -> actions
}

const defaultMaxActionsPerAgent = 2000

// NewRecorder creates a new empty Recorder.
func NewRecorder() *Recorder {
	return NewRecorderWithLimit(defaultMaxActionsPerAgent)
}

// NewRecorderWithLimit creates a Recorder that keeps the newest N actions per agent.
func NewRecorderWithLimit(maxActionsPerAgent int) *Recorder {
	if maxActionsPerAgent <= 0 {
		maxActionsPerAgent = defaultMaxActionsPerAgent
	}
	return &Recorder{
		maxActionsPerAgent: maxActionsPerAgent,
		actions:            make(map[string][]Action),
	}
}

// Record appends an action to the given agent's timeline.
func (r *Recorder) Record(agentID string, actionType ActionType, data json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()

	timeline := append(r.actions[agentID], Action{
		AgentID:   agentID,
		Type:      actionType,
		Timestamp: time.Now(),
		Data:      sanitizeActionData(data),
	})
	if len(timeline) > r.maxActionsPerAgent {
		kept := make([]Action, r.maxActionsPerAgent)
		copy(kept, timeline[len(timeline)-r.maxActionsPerAgent:])
		timeline = kept
	}
	r.actions[agentID] = timeline
}

func sanitizeActionData(data json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(data)) == 0 {
		return data
	}
	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return data
	}
	sanitized := sanitizeActionDataValue(payload, false)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return data
	}
	return encoded
}

func sanitizeActionDataValue(value interface{}, sensitiveContext bool) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		localSensitive := sensitiveContext
		for key, child := range typed {
			if sensitiveActionDataKey(key) || descriptorIdentifiesSensitiveField(key, child) {
				localSensitive = true
				break
			}
		}
		out := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			switch {
			case sensitiveActionDataKey(key):
				out[key] = "[redacted]"
			case localSensitive && typedActionValueKey(key):
				out[key] = "[redacted]"
			default:
				out[key] = sanitizeActionDataValue(child, localSensitive)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, child := range typed {
			out[i] = sanitizeActionDataValue(child, sensitiveContext)
		}
		return out
	default:
		return value
	}
}

func sensitiveActionDataKey(key string) bool {
	normalized := normalizeActionDataToken(key)
	for _, marker := range []string{"api_key", "apikey", "token", "secret", "password", "credential", "authorization", "cookie", "session"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "auth" || strings.HasSuffix(normalized, "_auth")
}

func descriptorIdentifiesSensitiveField(key string, value interface{}) bool {
	normalized := normalizeActionDataToken(key)
	switch normalized {
	case "selector", "field", "field_name", "name", "label", "placeholder", "aria_label", "type", "input_type":
	default:
		return false
	}
	raw, ok := value.(string)
	if !ok {
		return false
	}
	return sensitiveActionDataKey(raw)
}

func typedActionValueKey(key string) bool {
	switch normalizeActionDataToken(key) {
	case "text", "value", "input", "typed_text":
		return true
	default:
		return false
	}
}

func normalizeActionDataToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

// GetTimeline returns all recorded actions for the given agent, sorted by timestamp.
func (r *Recorder) GetTimeline(agentID string) []Action {
	r.mu.Lock()
	defer r.mu.Unlock()

	orig := r.actions[agentID]
	if len(orig) == 0 {
		return nil
	}

	// Return a copy sorted by time.
	result := make([]Action, len(orig))
	copy(result, orig)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})
	return result
}

// Export returns the agent's timeline as JSON bytes.
func (r *Recorder) Export(agentID string) ([]byte, error) {
	timeline := r.GetTimeline(agentID)
	if timeline == nil {
		timeline = []Action{}
	}
	return json.Marshal(timeline)
}

// Clear removes all recorded actions for the given agent.
func (r *Recorder) Clear(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.actions, agentID)
}

// actionToJugglerCall maps a recorded action to a Juggler method + params.
func actionToJugglerCall(a Action) (method string, params map[string]interface{}, err error) {
	var data map[string]interface{}
	if len(a.Data) > 0 {
		if err := json.Unmarshal(a.Data, &data); err != nil {
			return "", nil, fmt.Errorf("unmarshal action data: %w", err)
		}
	}
	if data == nil {
		data = make(map[string]interface{})
	}

	switch a.Type {
	case ActionNavigate:
		return "Page.navigate", data, nil
	case ActionClick:
		return "Page.dispatchMouseEvent", data, nil
	case ActionType_:
		return "Page.dispatchKeyEvent", data, nil
	case ActionScroll:
		return "Page.dispatchMouseEvent", data, nil
	case ActionScreenshot:
		return "Page.screenshot", data, nil
	case ActionEvaluate:
		return "Runtime.evaluate", data, nil
	case ActionSetContent:
		return "Page.setContent", data, nil
	default:
		return "", nil, fmt.Errorf("unknown action type %q", a.Type)
	}
}

// Replay executes all recorded actions for the given agent against a Juggler client.
// Actions are replayed in timestamp order with the original inter-action delays preserved.
func (r *Recorder) Replay(agentID string, client *juggler.Client, sessionID string) error {
	timeline := r.GetTimeline(agentID)
	if len(timeline) == 0 {
		return nil
	}

	for i, action := range timeline {
		// Preserve original timing gaps between actions.
		if i > 0 {
			gap := action.Timestamp.Sub(timeline[i-1].Timestamp)
			if gap > 0 {
				time.Sleep(gap)
			}
		}

		method, params, err := actionToJugglerCall(action)
		if err != nil {
			return fmt.Errorf("action %d (%s): %w", i, action.Type, err)
		}

		if _, err := client.Call(sessionID, method, params); err != nil {
			return fmt.Errorf("replay action %d (%s): %w", i, action.Type, err)
		}
	}

	return nil
}
