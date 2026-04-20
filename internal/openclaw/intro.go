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
	return "You are an AI agent named '" + name + "'. Your assigned runtime name for this session is exactly '" + name + "' and you must not claim a different name or inherited persona. Your task for this session is: " + task + ". Start with the task immediately. Do not introduce yourself or ask how you can help before taking the first concrete step. If the task asks for a specific reply or exact wording, return that output exactly."
}
