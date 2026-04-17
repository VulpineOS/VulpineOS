package openclaw

import "strings"

// IntroMessage returns the first-turn bootstrap prompt used for newly created agents.
func IntroMessage(name, task string) string {
	name = strings.TrimSpace(name)
	task = strings.TrimSpace(task)
	if task == "" {
		task = "Help with the assigned task."
	}
	if name == "" {
		name = "Agent"
	}
	return "You are an AI agent named '" + name + "'. Your assigned runtime name for this session is exactly '" + name + "' and you must not claim a different name or inherited persona. Your purpose: " + task + ". Introduce yourself briefly (1-2 sentences), use the assigned name exactly, and ask how you can help."
}
