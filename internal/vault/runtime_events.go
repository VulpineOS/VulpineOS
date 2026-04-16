package vault

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRuntimeEvents = 200
	minRuntimeEvents        = 25
	maxRuntimeEvents        = 5000
	runtimeRetentionKey     = "retention"
)

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

// RuntimeEventFilter selects recent runtime events.
type RuntimeEventFilter struct {
	Limit     int
	Component string
	Level     string
	Event     string
	Query     string
}

// RuntimeAuditSettings captures persisted audit settings.
type RuntimeAuditSettings struct {
	Retention int `json:"retention"`
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

	retention, err := db.runtimeEventRetention()
	if err != nil {
		return nil, err
	}
	if _, err := db.conn.Exec(
		`DELETE FROM runtime_events
		 WHERE id NOT IN (
		 	SELECT id FROM runtime_events ORDER BY timestamp DESC, id DESC LIMIT ?
		 )`,
		retention,
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

// ListRuntimeEvents returns recent runtime events first, with optional filtering.
func (db *DB) ListRuntimeEvents(filter RuntimeEventFilter) ([]RuntimeEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > maxRuntimeEvents {
		limit = maxRuntimeEvents
	}

	query := strings.Builder{}
	query.WriteString(`SELECT id, component, level, event, message, metadata, timestamp
		 FROM runtime_events
		 WHERE 1=1`)
	args := make([]interface{}, 0, 5)
	if value := strings.TrimSpace(filter.Component); value != "" {
		query.WriteString(` AND component = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Level); value != "" {
		query.WriteString(` AND level = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Event); value != "" {
		query.WriteString(` AND event = ?`)
		args = append(args, value)
	}
	if value := strings.TrimSpace(strings.ToLower(filter.Query)); value != "" {
		like := "%" + value + "%"
		query.WriteString(` AND (
			LOWER(component) LIKE ?
			OR LOWER(level) LIKE ?
			OR LOWER(event) LIKE ?
			OR LOWER(message) LIKE ?
			OR LOWER(metadata) LIKE ?
		)`)
		args = append(args, like, like, like, like, like)
	}
	query.WriteString(` ORDER BY timestamp DESC, id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := db.conn.Query(query.String(), args...)
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

// RuntimeAuditSettings returns persisted audit settings.
func (db *DB) RuntimeAuditSettings() (RuntimeAuditSettings, error) {
	retention, err := db.runtimeEventRetention()
	if err != nil {
		return RuntimeAuditSettings{}, err
	}
	return RuntimeAuditSettings{Retention: retention}, nil
}

// SetRuntimeAuditRetention persists the runtime event retention count.
func (db *DB) SetRuntimeAuditRetention(limit int) (RuntimeAuditSettings, error) {
	normalized := normalizeRuntimeRetention(limit)
	if _, err := db.conn.Exec(
		`INSERT INTO runtime_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		runtimeRetentionKey,
		strconv.Itoa(normalized),
	); err != nil {
		return RuntimeAuditSettings{}, fmt.Errorf("set runtime retention: %w", err)
	}
	if _, err := db.conn.Exec(
		`DELETE FROM runtime_events
		 WHERE id NOT IN (
		 	SELECT id FROM runtime_events ORDER BY timestamp DESC, id DESC LIMIT ?
		 )`,
		normalized,
	); err != nil {
		return RuntimeAuditSettings{}, fmt.Errorf("trim runtime events: %w", err)
	}
	return RuntimeAuditSettings{Retention: normalized}, nil
}

func (db *DB) runtimeEventRetention() (int, error) {
	if db == nil || db.conn == nil {
		return defaultMaxRuntimeEvents, nil
	}
	var raw string
	err := db.conn.QueryRow(
		`SELECT value FROM runtime_settings WHERE key = ?`,
		runtimeRetentionKey,
	).Scan(&raw)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return defaultMaxRuntimeEvents, nil
		}
		return 0, fmt.Errorf("load runtime retention: %w", err)
	}
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return defaultMaxRuntimeEvents, nil
	}
	return normalizeRuntimeRetention(limit), nil
}

func normalizeRuntimeRetention(limit int) int {
	switch {
	case limit <= 0:
		return defaultMaxRuntimeEvents
	case limit < minRuntimeEvents:
		return minRuntimeEvents
	case limit > maxRuntimeEvents:
		return maxRuntimeEvents
	default:
		return limit
	}
}
