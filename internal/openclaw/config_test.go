package openclaw

import "testing"

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
