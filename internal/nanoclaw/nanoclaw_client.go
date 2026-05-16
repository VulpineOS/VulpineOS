package nanoclaw

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

type NanoclawClient struct {
	socketPath string
}

func NewNanoclawClient(nanoclawDir string) *NanoclawClient {
	return &NanoclawClient{
		socketPath: filepath.Join(nanoclawDir, "data", "ncl.sock"),
	}
}

func (c *NanoclawClient) IsRunning() bool {
	_, err := os.Stat(c.socketPath)
	return err == nil
}

type NanoclawRequest struct {
	ID      string          `json:"id"`
	Command string          `json:"command"`
	Args    json.RawMessage `json:"args,omitempty"`
}

type NanoclawResponse struct {
	ID   string          `json:"id"`
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *NanoclawClient) SendMessage(sessionID, message string, onChunk func(string, bool)) error {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to nanoclaw: %w", err)
	}
	defer conn.Close()

	req := NanoclawRequest{
		ID:      generateRequestID(),
		Command: "chat",
		Args:    json.RawMessage(fmt.Sprintf(`{"sessionId":"%s","message":"%s"}`, sessionID, message)),
	}

	reqData, _ := json.Marshal(req)
	_, err = conn.Write(append(reqData, '\n'))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var resp NanoclawResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		if resp.Error != nil {
			return fmt.Errorf("nanoclaw error: %s", resp.Error.Message)
		}

		if resp.Data != nil {
			var chatResp struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Done bool `json:"done"`
			}
			if err := json.Unmarshal(resp.Data, &chatResp); err == nil {
				onChunk(chatResp.Message.Content, chatResp.Done)
				if chatResp.Done {
					return nil
				}
			}
		}
	}

	return nil
}

func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

func findNanoclawDir() string {
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 5; i++ {
		nanoclawDir := filepath.Join(dir, "nanoclaw")
		if _, err := os.Stat(filepath.Join(nanoclawDir, "data", "ncl.sock")); err == nil {
			return nanoclawDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func FindNanoclawSocket() (string, bool) {
	dir := findNanoclawDir()
	if dir == "" {
		return "", false
	}
	socketPath := filepath.Join(dir, "data", "ncl.sock")
	if _, err := os.Stat(socketPath); err == nil {
		return socketPath, true
	}
	return "", false
}