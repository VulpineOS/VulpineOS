package mcp

import (
	"fmt"
	"sync"
)

// labelIndex maps per-session human labels (e.g. "@3") to objectIDs
// returned by Page.getAnnotatedScreenshot. It is populated on every
// successful annotated screenshot capture so that agents can later
// call vulpine_click_label to click an element by label rather than
// juggling raw objectIDs.
type labelIndex struct {
	mu       sync.RWMutex
	sessions map[string]map[string]string // sessionID -> label -> objectID
}

// Set replaces the label map for sessionID with the labels derived
// from elements. Each element is expected to be a map with at least a
// "label" (string) and "objectId" (string) field. Elements missing
// either field are skipped.
func (l *labelIndex) Set(sessionID string, elements []map[string]interface{}) {
	next := map[string]string{}
	for i, el := range elements {
		label, _ := el["label"].(string)
		if label == "" {
			// Synthesize @N if the backend didn't supply one.
			label = fmt.Sprintf("@%d", i+1)
		}
		if obj, ok := el["objectId"].(string); ok && obj != "" {
			next[label] = obj
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sessions[sessionID] = next
}

// Get returns the objectID mapped to label in sessionID, or ok=false
// if no such label is known.
func (l *labelIndex) Get(sessionID, label string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	session, ok := l.sessions[sessionID]
	if !ok {
		return "", false
	}
	obj, ok := session[label]
	return obj, ok
}

// Clear drops any label mappings for sessionID. Intended for use on
// navigation or context teardown so stale objectIDs don't leak.
func (l *labelIndex) Clear(sessionID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sessions, sessionID)
}

// globalLabels is the process-wide label index shared between the
// annotated screenshot tool and vulpine_click_label.
var globalLabels = &labelIndex{sessions: map[string]map[string]string{}}
