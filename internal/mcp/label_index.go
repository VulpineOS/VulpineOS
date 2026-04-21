package mcp

import (
	"fmt"
	"sync"
	"time"
)

// MaxLabelSessions is the upper bound on distinct session entries the
// label index retains. When Set would exceed this cap the least
// recently accessed session is evicted. This keeps long-lived
// processes from accumulating stale objectID maps forever.
const MaxLabelSessions = 256

// labelIndex maps per-session human labels (e.g. "@3") to objectIDs
// returned by Page.getAnnotatedScreenshot. It is populated on every
// successful annotated screenshot capture so that agents can later
// call vulpine_click_label to click an element by label rather than
// juggling raw objectIDs. It is bounded to MaxLabelSessions entries
// via an LRU policy on last access time.
type labelTarget struct {
	ObjectID string
	FrameID  string
}

type labelIndex struct {
	mu         sync.RWMutex
	sessions   map[string]map[string]labelTarget // sessionID -> label -> target
	lastAccess map[string]time.Time              // sessionID -> last touch time
}

// touchLocked updates the access time for sessionID. Caller must hold
// the write lock.
func (l *labelIndex) touchLocked(sessionID string) {
	if l.lastAccess == nil {
		l.lastAccess = map[string]time.Time{}
	}
	l.lastAccess[sessionID] = time.Now()
}

// evictOldestLocked drops the single least recently accessed session.
// Caller must hold the write lock.
func (l *labelIndex) evictOldestLocked() {
	var oldestID string
	var oldest time.Time
	first := true
	for id, t := range l.lastAccess {
		if first || t.Before(oldest) {
			oldest = t
			oldestID = id
			first = false
		}
	}
	if oldestID != "" {
		delete(l.sessions, oldestID)
		delete(l.lastAccess, oldestID)
	}
}

// Set replaces the label map for sessionID with the labels derived
// from elements. Each element is expected to be a map with at least a
// "label" (string) and "objectId" (string) field. Elements missing
// either field are skipped. If the session count would exceed
// MaxLabelSessions the least recently used session is evicted first.
func (l *labelIndex) Set(sessionID string, elements []map[string]interface{}) {
	next := map[string]labelTarget{}
	for i, el := range elements {
		label, _ := el["label"].(string)
		if label == "" {
			// Synthesize @N if the backend didn't supply one.
			label = fmt.Sprintf("@%d", i+1)
		}
		if obj, ok := el["objectId"].(string); ok && obj != "" {
			frameID, _ := el["frameId"].(string)
			next[label] = labelTarget{ObjectID: obj, FrameID: frameID}
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.sessions[sessionID]; !exists {
		for len(l.sessions) >= MaxLabelSessions {
			l.evictOldestLocked()
		}
	}
	l.sessions[sessionID] = next
	l.touchLocked(sessionID)
}

// Get returns the target mapped to label in sessionID, or ok=false
// if no such label is known. Touches the session access time on hit.
func (l *labelIndex) Get(sessionID, label string) (labelTarget, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	session, ok := l.sessions[sessionID]
	if !ok {
		return labelTarget{}, false
	}
	target, ok := session[label]
	if ok {
		l.touchLocked(sessionID)
	}
	return target, ok
}

// Clear drops any label mappings for sessionID. Intended for use on
// navigation or context teardown so stale objectIDs don't leak.
func (l *labelIndex) Clear(sessionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sessions, sessionID)
	delete(l.lastAccess, sessionID)
}

// Len returns the current number of tracked sessions. Intended for
// tests and metrics.
func (l *labelIndex) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.sessions)
}

// globalLabels is the process-wide label index shared between the
// annotated screenshot tool and vulpine_click_label.
var globalLabels = &labelIndex{
	sessions:   map[string]map[string]labelTarget{},
	lastAccess: map[string]time.Time{},
}
