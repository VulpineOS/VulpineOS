package opencode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type Client struct {
	binaryPath string
	sessionID  string
	mu         sync.Mutex
}

type TextPart struct {
	Text string `json:"text"`
}

type TextEvent struct {
	Type string    `json:"type"`
	Part TextPart  `json:"part"`
}

type Tokens struct {
	Total  int `json:"total"`
	Input  int `json:"input"`
	Output int `json:"output"`
}

type StepFinishPart struct {
	Tokens Tokens `json:"tokens"`
}

type StepFinishEvent struct {
	Type string          `json:"type"`
	Part StepFinishPart  `json:"part"`
}

func NewClient(binaryPath string) *Client {
	if binaryPath == "" {
		binaryPath = "opencode"
	}
	return &Client{
		binaryPath: binaryPath,
	}
}

func (c *Client) Start() error {
	return nil
}

func (c *Client) SetSession(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sessionID
}

func (c *Client) SendMessage(prompt string) (string, int, error) {
	c.mu.Lock()
	sessionID := c.sessionID
	c.mu.Unlock()

	args := []string{"run", "--format", "json"}
	if sessionID != "" {
		args = append(args, "-s", sessionID, "-c")
	}
	args = append(args, prompt)

	cmd := exec.Command(c.binaryPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("start opencode: %w", err)
	}

	defer cmd.Wait()

	response, tokens, parseErr := c.parseResponse(stdout)

	if sessionID == "" {
		c.mu.Lock()
		c.sessionID = extractSessionID(stdout)
		c.mu.Unlock()
	}

	return response, tokens, parseErr
}

func extractSessionID(stdout io.Reader) string {
	return ""
}

func (c *Client) parseResponse(stdout io.Reader) (string, int, error) {
	scanner := bufio.NewScanner(stdout)
	var fullText string
	var totalTokens int
	var foundText bool

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var textEvent TextEvent
		if err := json.Unmarshal([]byte(line), &textEvent); err == nil {
			if textEvent.Type == "text" {
				fullText += textEvent.Part.Text
				foundText = true
			}
			continue
		}

		var stepFinishEvent StepFinishEvent
		if err := json.Unmarshal([]byte(line), &stepFinishEvent); err == nil {
			if stepFinishEvent.Type == "step_finish" {
				totalTokens = stepFinishEvent.Part.Tokens.Total
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", 0, fmt.Errorf("error reading response: %w", err)
	}

	if !foundText {
		return "", 0, fmt.Errorf("no response from opencode")
	}

	return strings.TrimSpace(fullText), totalTokens, nil
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) IsRunning() bool {
	return true
}