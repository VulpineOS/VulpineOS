package vault

import (
	"encoding/json"
	"fmt"
	"time"
)

const maxRuntimeEvents = 200

// RuntimeEvent is an operator-facing runtime lifecycle event.
type RuntimeEvent struct {
	ID        int               `json:"id"`
	Component string            `json:"component"`
	Level     string            `json:"level"`
	Event     string            `json:"event"`
	Message   string            `json:"message"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// AppendRuntimeEvent stores a runtime event and trims the retained history.
func (db *DB) AppendRuntimeEvent(component, level, event, message string, metadata map[string]string) (*RuntimeEvent, error) {
	if component == "" {
		return nil, fmt.Errorf("component is required")
	}
	if level == "" {
		level = "info"
	}
	if event == "" {
		return nil, fmt.Errorf("event is required")
	}
	if message == "" {
		message = event
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime event metadata: %w", err)
	}

	now := time.Now().Unix()
	result, err := db.conn.Exec(
		`INSERT INTO runtime_events (component, level, event, message, metadata, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		component, level, event, message, string(encoded), now,
	)
	if err != nil {
		return nil, fmt.Errorf("append runtime event: %w", err)
	}
	if _, err := db.conn.Exec(
		`DELETE FROM runtime_events
		 WHERE id NOT IN (
		 	SELECT id FROM runtime_events ORDER BY timestamp DESC, id DESC LIMIT ?
		 )`,
		maxRuntimeEvents,
	); err != nil {
		return nil, fmt.Errorf("trim runtime events: %w", err)
	}

	id, _ := result.LastInsertId()
	return &RuntimeEvent{
		ID:        int(id),
		Component: component,
		Level:     level,
		Event:     event,
		Message:   message,
		Metadata:  metadata,
		Timestamp: time.Unix(now, 0),
	}, nil
}

// ListRuntimeEvents returns the most recent runtime events first.
func (db *DB) ListRuntimeEvents(limit int) ([]RuntimeEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.Query(
		`SELECT id, component, level, event, message, metadata, timestamp
		 FROM runtime_events
		 ORDER BY timestamp DESC, id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list runtime events: %w", err)
	}
	defer rows.Close()

	events := make([]RuntimeEvent, 0, limit)
	for rows.Next() {
		var event RuntimeEvent
		var rawMetadata string
		var ts int64
		if err := rows.Scan(&event.ID, &event.Component, &event.Level, &event.Event, &event.Message, &rawMetadata, &ts); err != nil {
			return nil, fmt.Errorf("scan runtime event: %w", err)
		}
		if rawMetadata != "" {
			_ = json.Unmarshal([]byte(rawMetadata), &event.Metadata)
		}
		if event.Metadata == nil {
			event.Metadata = map[string]string{}
		}
		event.Timestamp = time.Unix(ts, 0)
		events = append(events, event)
	}
	return events, nil
}
