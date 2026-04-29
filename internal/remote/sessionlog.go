package remote

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

const redactedReasoningText = "[redacted hidden reasoning]"
const maxPanelSessionLogBytes int64 = 1 << 20

type sessionLogPanelMeta struct {
	truncated  bool
	bytes      int64
	totalBytes int64
}

func readSessionLogPanelContent(path string) (string, sessionLogPanelMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", sessionLogPanelMeta{}, err
	}
	meta := sessionLogPanelMeta{totalBytes: info.Size()}
	if info.Size() == 0 {
		return "", meta, nil
	}

	start := int64(0)
	if info.Size() > maxPanelSessionLogBytes {
		start = info.Size() - maxPanelSessionLogBytes
		meta.truncated = true
	}

	file, err := os.Open(path)
	if err != nil {
		return "", meta, err
	}
	defer file.Close()

	if start > 0 {
		if _, err := file.Seek(start, io.SeekStart); err != nil {
			return "", meta, err
		}
	}

	data, err := io.ReadAll(io.LimitReader(file, maxPanelSessionLogBytes))
	if err != nil {
		return "", meta, err
	}
	if meta.truncated {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		} else {
			data = nil
		}
	}
	meta.bytes = int64(len(data))
	return sanitizeSessionLog(string(data)), meta, nil
}

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
