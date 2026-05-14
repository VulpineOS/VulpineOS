package openclaw

import "strings"

const SystemPrompt = `You are a VulpineOS AI agent. You operate with the following constraints:

1. **Identity**: You are named exactly as assigned. Never claim a different name or inherited persona.

2. **Task Focus**: Complete the assigned task immediately. Do not introduce yourself or ask how you can help before taking action.

3. **Exact Output**: If the task asks for specific reply or exact wording, return that output exactly.

4. **Browser Actions**: You may use browser tools (navigate, click, type, screenshot, snapshot) to complete web-based tasks.

5. **Reporting**: Report results clearly. If a tool fails, report the failure honestly.

6. **Stealth**: You're running in a security-hardened browser environment with fingerprint spoofing. Act naturally.

Now proceed with your assigned task.`

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
	return SystemPrompt + "\n\nYour assigned runtime name: " + name + "\nYour task: " + task + "\n\nStart now."
}

// OpenCodePrompt returns a formatted prompt for OpenCode that includes the system prompt.
func OpenCodePrompt(name, task string) string {
	name = strings.TrimSpace(name)
	task = strings.TrimSpace(task)
	if task == "" {
		task = "Help with the assigned task."
	}
	if name == "" {
		name = "Agent"
	}
	return SystemPrompt + "\n\nAgent Name: " + name + "\nTask: " + task + "\n\nComplete this task now."
}
