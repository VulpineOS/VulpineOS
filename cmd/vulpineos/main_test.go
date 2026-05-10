package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/remote"
	"vulpineos/internal/testutil"
	"vulpineos/internal/vault"
)

var errStopAfterFlagParse = errors.New("stop after flag parse")

// TestRun_VersionFlag verifies the Run wrapper-friendly entrypoint
// honors --version, writes a version string to the configured stdout,
// and returns exit code 0. This is the contract that an alternate
// front-end binary depends on when delegating to Run(os.Args).
func TestRun_VersionFlag(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	prevOut, prevErr := stdout, stderr
	stdout = &outBuf
	stderr = &errBuf
	t.Cleanup(func() {
		stdout = prevOut
		stderr = prevErr
	})

	code := Run([]string{"vulpineos", "--version"})
	if code != 0 {
		t.Fatalf("Run(--version) exit code = %d, want 0 (stderr=%q)", code, errBuf.String())
	}
	out := outBuf.String()
	if !strings.Contains(out, "vulpineos") {
		t.Errorf("Run(--version) stdout = %q, expected to contain %q", out, "vulpineos")
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("Run(--version) produced empty stdout")
	}
}

func TestRun_HelpFlagsExitZero(t *testing.T) {
	for _, args := range [][]string{
		{"vulpineos", "--help"},
		{"vulpineos", "tui", "--help"},
		{"vulpineos", "panel", "--help"},
		{"vulpineos", "serve", "--help"},
		{"vulpineos", "remote", "--help"},
		{"vulpineos", "mcp", "--help"},
	} {
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			var outBuf, errBuf bytes.Buffer

			prevOut, prevErr := stdout, stderr
			stdout = &outBuf
			stderr = &errBuf
			t.Cleanup(func() {
				stdout = prevOut
				stderr = prevErr
			})

			if code := Run(args); code != 0 {
				t.Fatalf("Run(%v) exit code = %d, want 0 (stderr=%q)", args, code, errBuf.String())
			}
			if strings.TrimSpace(errBuf.String()) == "" {
				t.Fatalf("Run(%v) should print help text to stderr", args)
			}
		})
	}
}

func TestRun_LocalTUIDefaultsHeadlessAndSupportsHeadful(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantHeadless bool
	}{
		{
			name:         "bare command defaults local TUI to headless",
			args:         []string{"vulpineos"},
			wantHeadless: true,
		},
		{
			name:         "bare command headful flag launches visible browser",
			args:         []string{"vulpineos", "--headful"},
			wantHeadless: false,
		},
		{
			name:         "tui subcommand defaults to headless",
			args:         []string{"vulpineos", "tui"},
			wantHeadless: true,
		},
		{
			name:         "tui subcommand headful flag launches visible browser",
			args:         []string{"vulpineos", "tui", "--headful"},
			wantHeadless: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeadless bool
			called := false

			prevRunLocal := runLocalSession
			runLocalSession = func(binaryPath string, headless bool, profileDir string, noBrowser bool) error {
				called = true
				gotHeadless = headless
				return errStopAfterFlagParse
			}
			t.Cleanup(func() {
				runLocalSession = prevRunLocal
			})

			var outBuf, errBuf bytes.Buffer
			prevOut, prevErr := stdout, stderr
			stdout = &outBuf
			stderr = &errBuf
			t.Cleanup(func() {
				stdout = prevOut
				stderr = prevErr
			})

			if code := Run(tt.args); code != 1 {
				t.Fatalf("Run(%v) exit code = %d, want 1 from stub error", tt.args, code)
			}
			if !called {
				t.Fatalf("Run(%v) did not call local TUI startup", tt.args)
			}
			if gotHeadless != tt.wantHeadless {
				t.Fatalf("Run(%v) headless = %v, want %v", tt.args, gotHeadless, tt.wantHeadless)
			}
			if !strings.Contains(errBuf.String(), errStopAfterFlagParse.Error()) {
				t.Fatalf("Run(%v) stderr = %q, want stub error", tt.args, errBuf.String())
			}
		})
	}
}

func TestStartLocalSessionLoggingWritesToFile(t *testing.T) {
	restore, path := startLocalSessionLogging(t.TempDir())
	if path == "" {
		t.Fatal("startLocalSessionLogging returned empty path")
	}

	log.Print("startup log redirected")
	restore()

	if filepath.Base(path) != "local-tui.log" {
		t.Fatalf("log path = %q, want local-tui.log", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "startup log redirected") {
		t.Fatalf("log file %q missing redirected message: %q", path, string(data))
	}
}

func TestOpenServerVaultReportsStartupIssue(t *testing.T) {
	prevOpenVault := openVault
	openVault = func() (*vault.DB, error) {
		return nil, errors.New("permission denied")
	}
	t.Cleanup(func() {
		openVault = prevOpenVault
	})

	db, audit, issues, cleanup := openServerVault()
	t.Cleanup(cleanup)
	if db != nil {
		t.Fatal("db = non-nil, want nil on open failure")
	}
	if audit != nil {
		t.Fatal("audit = non-nil, want nil on open failure")
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %#v, want one issue", issues)
	}
	if issues[0].Component != "vault" || issues[0].Level != "error" || !strings.Contains(issues[0].Message, "permission denied") {
		t.Fatalf("unexpected startup issue: %#v", issues[0])
	}
}

func TestServerContextRegistryReplaysExistingTargetsAfterWiring(t *testing.T) {
	transport := testutil.NewFakeJugglerTransport(t)
	transport.RespondFunc("Browser.enable", func(msg *juggler.Message) (json.RawMessage, *juggler.Error) {
		params := testutil.ParamsAs[struct {
			AttachToDefaultContext bool `json:"attachToDefaultContext"`
		}](t, msg.Params)
		if params.AttachToDefaultContext {
			transport.InjectEvent("", "Browser.attachedToTarget", map[string]any{
				"sessionId": "session-1",
				"targetInfo": map[string]any{
					"browserContextId": "context-1",
					"url":              "https://example.com",
				},
			})
		}
		return json.RawMessage(`{}`), nil
	})
	client := juggler.NewClient(transport)
	t.Cleanup(func() { _ = client.Close() })

	registry := remote.NewContextRegistry()
	wireServerBrowserEvents(client, registry, func(method string, sessionID string, params json.RawMessage) {})
	if err := replayServerBrowserTargets(client); err != nil {
		t.Fatalf("replayServerBrowserTargets: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		contexts := registry.List()
		if len(contexts) == 1 && contexts[0].ID == "context-1" && contexts[0].Pages == 1 && contexts[0].LastURL == "https://example.com" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("registry contexts = %+v, want replayed context-1", contexts)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNormalizeRemotePanelURL(t *testing.T) {
	got, err := normalizeRemotePanelURL("wss://example.com:8443/ws", "secret")
	if err != nil {
		t.Fatalf("normalizeRemotePanelURL error: %v", err)
	}
	want := "https://example.com:8443/?token=secret"
	if got != want {
		t.Fatalf("normalizeRemotePanelURL = %q, want %q", got, want)
	}
}

func TestNormalizeRemotePanelURL_DefaultLocalURL(t *testing.T) {
	got, err := normalizeRemotePanelURL("", "secret")
	if err != nil {
		t.Fatalf("normalizeRemotePanelURL default error: %v", err)
	}
	want := "http://127.0.0.1:8443/?token=secret"
	if got != want {
		t.Fatalf("normalizeRemotePanelURL default = %q, want %q", got, want)
	}
}

func TestNormalizeRemotePanelDisplayURLStripsToken(t *testing.T) {
	got, err := normalizeRemotePanelDisplayURL("wss://example.com:8443/ws?token=secret&view=agents")
	if err != nil {
		t.Fatalf("normalizeRemotePanelDisplayURL error: %v", err)
	}
	want := "https://example.com:8443/?view=agents"
	if got != want {
		t.Fatalf("normalizeRemotePanelDisplayURL = %q, want %q", got, want)
	}
}

func TestNormalizeRemoteTUIURL(t *testing.T) {
	got, err := normalizeRemoteTUIURL("https://example.com:8443")
	if err != nil {
		t.Fatalf("normalizeRemoteTUIURL error: %v", err)
	}
	want := "wss://example.com:8443/ws"
	if got != want {
		t.Fatalf("normalizeRemoteTUIURL = %q, want %q", got, want)
	}
}

func TestEnsureAccessKeyGeneratesWhenMissing(t *testing.T) {
	key, generated, err := ensureAccessKey("")
	if err != nil {
		t.Fatalf("ensureAccessKey error: %v", err)
	}
	if !generated {
		t.Fatal("expected generated key")
	}
	if len(key) != 32 {
		t.Fatalf("generated key length = %d, want 32", len(key))
	}
}

func TestBuildPanelURLRewritesWildcardHost(t *testing.T) {
	got := buildPanelURL("0.0.0.0", 8443, false, "secret")
	want := "http://localhost:8443/?token=secret"
	if got != want {
		t.Fatalf("buildPanelURL = %q, want %q", got, want)
	}
}

func TestBuildPanelURLBracketsIPv6Host(t *testing.T) {
	for _, host := range []string{"::1", "[::1]"} {
		got := buildPanelURL(host, 8443, true, "secret")
		want := "https://[::1]:8443/?token=secret"
		if got != want {
			t.Fatalf("buildPanelURL(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestPrintPanelAccessGeneratedKey(t *testing.T) {
	var outBuf bytes.Buffer

	prevOut := stdout
	stdout = &outBuf
	t.Cleanup(func() {
		stdout = prevOut
	})

	got := printPanelAccess("0.0.0.0", 8443, true, "secret", true)
	wantURL := "https://localhost:8443/?token=secret"
	if got != wantURL {
		t.Fatalf("printPanelAccess URL = %q, want %q", got, wantURL)
	}

	out := outBuf.String()
	for _, want := range []string{
		"Listening on: 0.0.0.0:8443",
		"Panel URL: https://localhost:8443/",
		"API key: secret (generated)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("printPanelAccess output %q missing %q", out, want)
		}
	}
	if strings.Contains(out, "?token=secret") {
		t.Fatalf("generated-key output leaked token URL: %q", out)
	}
}

func TestPrintPanelAccessExplicitKeyAvoidsTokenURL(t *testing.T) {
	var outBuf bytes.Buffer

	prevOut := stdout
	stdout = &outBuf
	t.Cleanup(func() {
		stdout = prevOut
	})

	got := printPanelAccess("127.0.0.1", 8443, false, "secret", false)
	if got != "http://127.0.0.1:8443/?token=secret" {
		t.Fatalf("printPanelAccess URL = %q", got)
	}

	out := outBuf.String()
	if strings.Contains(out, "?token=secret") {
		t.Fatalf("explicit-key output leaked token URL: %q", out)
	}
	for _, want := range []string{
		"Panel URL: http://127.0.0.1:8443/",
		"API key: secret",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("printPanelAccess output %q missing %q", out, want)
		}
	}
}

func TestTUIProgramsEnableMouseCellMotion(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	funcs := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok {
			funcs[fn.Name.Name] = fn
		}
	}

	for _, name := range []string{"runLocal", "runRemote"} {
		fn := funcs[name]
		if fn == nil {
			t.Fatalf("missing %s", name)
		}
		foundAppProgram := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSelectorCall(call.Fun, "tea", "NewProgram") || len(call.Args) == 0 {
				return true
			}
			first, ok := call.Args[0].(*ast.Ident)
			if !ok || first.Name != "app" {
				return true
			}
			foundAppProgram = true
			if !callHasSelectorArg(call, "tea", "WithMouseCellMotion") {
				t.Errorf("%s app tea.NewProgram missing tea.WithMouseCellMotion()", name)
			}
			return true
		})
		if !foundAppProgram {
			t.Fatalf("%s did not create app tea.NewProgram", name)
		}
	}
}

func isSelectorCall(expr ast.Expr, pkg string, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

func callHasSelectorArg(call *ast.CallExpr, pkg string, name string) bool {
	for _, arg := range call.Args {
		option, ok := arg.(*ast.CallExpr)
		if ok && isSelectorCall(option.Fun, pkg, name) {
			return true
		}
	}
	return false
}

func TestRunRemoteRejectsUnknownMode(t *testing.T) {
	var outBuf, errBuf bytes.Buffer

	prevOut, prevErr := stdout, stderr
	stdout = &outBuf
	stderr = &errBuf
	t.Cleanup(func() {
		stdout = prevOut
		stderr = prevErr
	})

	code := runRemoteSubcommand([]string{"foo"})
	if code != 2 {
		t.Fatalf("runRemoteSubcommand unknown mode code = %d, want 2", code)
	}
	if !strings.Contains(errBuf.String(), `unknown remote mode "foo"`) {
		t.Fatalf("stderr = %q", errBuf.String())
	}
}
