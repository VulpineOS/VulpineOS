package nanoclaw

import (
	"os"
	"testing"
)

func TestGenerateConfigUsesCurrentMCPCommand(t *testing.T) {
	cfg, err := GenerateConfig("/usr/local/bin/vulpineos", "ws://127.0.0.1:8443/ws")
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	server, ok := cfg.Plugins.MCP.Servers["vulpineos"]
	if !ok {
		t.Fatal("missing vulpineos MCP server")
	}
	want := []string{"mcp", "--connect", "ws://127.0.0.1:8443/ws"}
	if len(server.Args) != len(want) {
		t.Fatalf("args = %#v, want %#v", server.Args, want)
	}
	for i := range want {
		if server.Args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", server.Args, want)
		}
	}
}

func TestWriteConfigUsesPrivatePermissions(t *testing.T) {
	cfg, err := GenerateConfig("/usr/local/bin/vulpineos", "ws://127.0.0.1:8443/ws")
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}

	path, err := WriteConfig(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config mode = %o, want 0600", got)
	}
}
