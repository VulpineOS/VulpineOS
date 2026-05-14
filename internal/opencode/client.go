package opencode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Client struct {
	binaryPath string
	serverURL  string
	cmd        *exec.Cmd
	mu         sync.Mutex
	started    bool
	client     *http.Client
}

type MessageRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
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
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *Client) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	cmd := exec.Command(c.binaryPath, "serve", "--port", "0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start opencode server: %w", err)
	}

	c.cmd = cmd

	if err := c.waitForServer(); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("server failed to start: %w", err)
	}

	c.started = true
	return nil
}

func (c *Client) waitForServer() error {
	maxAttempts := 30
	checkInterval := 500 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		time.Sleep(checkInterval)

		if c.cmd.Process == nil {
			continue
		}

		resp, err := c.client.Get("http://127.0.0.1:4096/");
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
			c.serverURL = "http://127.0.0.1:4096"
			return nil
		}

		ports := []string{"4096", "4097", "4098", "4099", "4100"}
		for _, port := range ports {
			resp, err := c.client.Get(fmt.Sprintf("http://127.0.0.1:%s/", port))
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
					c.serverURL = fmt.Sprintf("http://127.0.0.1:%s", port)
					return nil
				}
			}
		}
	}

	return fmt.Errorf("server did not become ready in time")
}

func (c *Client) SendMessage(prompt string) (string, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		if err := c.Start(); err != nil {
			return "", 0, err
		}
	}

	reqBody, err := json.Marshal(MessageRequest{Message: prompt})
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.client.Post(c.serverURL+"/api/message", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return c.parseResponse(resp.Body)
}

func (c *Client) parseResponse(body io.Reader) (string, int, error) {
	scanner := bufio.NewScanner(body)
	var fullText string
	var totalTokens int

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var textEvent TextEvent
		if err := json.Unmarshal([]byte(line), &textEvent); err == nil {
			if textEvent.Type == "text" {
				fullText += textEvent.Part.Text
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

	return fullText, totalTokens, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	c.started = false
	return nil
}

func (c *Client) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started && c.cmd != nil && c.cmd.Process != nil
}