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
		socketPath: filepath.Join(nanoclawDir, "data", "cli.sock"),
	}
}

func (c *NanoclawClient) IsRunning() bool {
	_, err := os.Stat(c.socketPath)
	return err == nil
}

func (c *NanoclawClient) SendMessage(message string, onChunk func(string, bool)) error {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to nanoclaw CLI socket: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(fmt.Sprintf(`{"text":"%s"}`+"\n", message)))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	reader := bufio.NewReader(conn)
	silenceTimer := time.AfterFunc(2*time.Second, func() {
		onChunk("", true)
		conn.Close()
	})

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		if text, ok := msg["text"].(string); ok && text != "" {
			silenceTimer.Stop()
			onChunk(text, false)
			silenceTimer.Reset(2 * time.Second)
		}
	}

	return nil
}

func findNanoclawDir() string {
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 5; i++ {
		nanoclawDir := filepath.Join(dir, "nanoclaw")
		if _, err := os.Stat(filepath.Join(nanoclawDir, "data", "cli.sock")); err == nil {
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
	socketPath := filepath.Join(dir, "data", "cli.sock")
	if _, err := os.Stat(socketPath); err == nil {
		return socketPath, true
	}
	return "", false
}

func GetNanoclawDir() string {
	return findNanoclawDir()
}