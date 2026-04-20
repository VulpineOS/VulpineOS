package remote

import (
	"encoding/json"
	"strings"
)

const redactedReasoningText = "[redacted hidden reasoning]"

func sanitizeSessionLog(raw string) string {
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = sanitizeSessionLogLine(line)
	}
	return strings.Join(lines, "\n")
}

func sanitizeSessionLogLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return line
	}

	sanitized := sanitizeSessionLogValue(payload)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return line
	}
	return string(encoded)
}

func sanitizeSessionLogValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		delete(typed, "thinkingSignature")
		delete(typed, "reasoningSignature")

		if _, ok := typed["thinking"]; ok {
			if kind, _ := typed["type"].(string); kind == "thinking" {
				typed["thinking"] = redactedReasoningText
			}
		}
		if _, ok := typed["reasoning"]; ok {
			typed["reasoning"] = redactedReasoningText
		}
		if _, ok := typed["reasoning_content"]; ok {
			typed["reasoning_content"] = redactedReasoningText
		}
		if _, ok := typed["reasoningContent"]; ok {
			typed["reasoningContent"] = redactedReasoningText
		}

		for key, child := range typed {
			typed[key] = sanitizeSessionLogValue(child)
		}
		return typed
	case []interface{}:
		for i, child := range typed {
			typed[i] = sanitizeSessionLogValue(child)
		}
		return typed
	default:
		return value
	}
}
