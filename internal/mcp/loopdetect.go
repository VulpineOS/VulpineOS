package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// LoopDetector tracks consecutive identical tool calls per session.
// When the same tool+args combination is called N times in a row,
// it returns a warning asking the agent to try a different approach.
type LoopDetector struct {
	mu       sync.Mutex
	history  map[string][]string // sessionID → list of recent action hashes
	maxRepeat int                // consecutive identical actions before warning
}

func NewLoopDetector(maxRepeat int) *LoopDetector {
	if maxRepeat <= 0 {
		maxRepeat = 3
	}
	return &LoopDetector{
		history:   make(map[string][]string),
		maxRepeat: maxRepeat,
	}
}

// Check returns a warning message if a loop is detected, or empty string if ok.
func (ld *LoopDetector) Check(sessionID, toolName, argsStr string) string {
	ld.mu.Lock()
	defer ld.mu.Unlock()

	hash := hashAction(toolName, argsStr)
	history := ld.history[sessionID]

	// Count consecutive identical actions at the end of history
	consecutive := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i] == hash {
			consecutive++
		} else {
			break
		}
	}

	// Add current action to history
	history = append(history, hash)
	// Keep only last 20 actions
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	ld.history[sessionID] = history

	if consecutive >= ld.maxRepeat {
		return fmt.Sprintf("WARNING: You have called %s with the same arguments %d times in a row. This action is not making progress. Try a different approach: use vulpine_find to locate the correct element, vulpine_verify to check page state, or vulpine_page_info to reassess the situation.", toolName, consecutive+1)
	}

	return ""
}

// Reset clears history for a session (e.g. after navigation to a new page).
func (ld *LoopDetector) Reset(sessionID string) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	delete(ld.history, sessionID)
}

func hashAction(tool, args string) string {
	h := sha256.Sum256([]byte(tool + ":" + args))
	return hex.EncodeToString(h[:8])
}
