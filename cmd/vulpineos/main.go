package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"vulpineos/internal/agentbus"
	"vulpineos/internal/config"
	"vulpineos/internal/costtrack"
	"vulpineos/internal/extensions"
	"vulpineos/internal/foxbridge"
	"vulpineos/internal/juggler"
	"vulpineos/internal/kernel"
	"vulpineos/internal/mcp"
	"vulpineos/internal/openclaw"
	"vulpineos/internal/orchestrator"
	"vulpineos/internal/pagecache"
	"vulpineos/internal/pool"
	"vulpineos/internal/proxy"
	"vulpineos/internal/recording"
	"vulpineos/internal/remote"
	"vulpineos/internal/runtimeaudit"
	"vulpineos/internal/tui"
	"vulpineos/internal/tui/loading"
	"vulpineos/internal/tui/setup"
	"vulpineos/internal/vault"
	"vulpineos/internal/webhooks"
)

// Version is the VulpineOS build version string reported by --version.
// It is overridable via -ldflags "-X main.Version=..." at build time.
var Version = "dev"

// stdout and stderr are indirections so that Run can be driven from a
// test with a captured buffer. Production code uses the package-level
// os.Stdout/os.Stderr by default.
var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

var startGatewayIfAvailable = func(cfg *config.Config, audit *runtimeaudit.Manager) *openclaw.Gateway {
	mgr := openclaw.NewManager("")
	if !mgr.OpenClawInstalled() {
		return nil
	}
	gw := openclaw.NewGateway("")
	if err := gw.Start(); err != nil {
		log.Printf("Warning: OpenClaw gateway failed to start: %v (browser tools won't work)", err)
		if audit != nil {
			_, _ = audit.Log("gateway", "error", "start_failed", "OpenClaw gateway failed to start", map[string]string{
				"error": err.Error(),
			})
		}
		return nil
	}
	if audit != nil {
		_, _ = audit.Log("gateway", "info", "started", "OpenClaw gateway started", nil)
	}
	if cfg != nil {
		if err := config.RepairOpenClawProfile(cfg.FoxbridgeCDPURL); err != nil {
			log.Printf("Warning: could not repair OpenClaw profile after gateway start: %v", err)
			if audit != nil {
				_, _ = audit.Log("gateway", "warn", "profile_repair_failed", "OpenClaw profile repair failed after gateway start", map[string]string{
					"error": err.Error(),
				})
			}
		}
	}
	return gw
}

func startLocalSessionLogging(baseDir string) (restore func(), path string) {
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()

	restore = func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	}

	logDir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		log.SetOutput(io.Discard)
		return restore, ""
	}

	path = filepath.Join(logDir, "local-tui.log")
	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		log.SetOutput(io.Discard)
		return restore, ""
	}

	log.SetOutput(logFile)
	restore = func() {
		_ = logFile.Close()
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	}
	return restore, path
}

func enableBrowser(client *juggler.Client, label string) error {
	var lastErr error
	time.Sleep(1 * time.Second)
	for attempt := 0; attempt < 5; attempt++ {
		_, err := client.Call("", "Browser.enable", map[string]interface{}{
			"attachToDefaultContext": true,
		})
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return fmt.Errorf("%s: %w", label, lastErr)
}

func main() {
	os.Exit(Run(os.Args))
}

// Run is the wrapper-friendly entrypoint for the VulpineOS CLI. It
// parses the given argv (including the program name in args[0]) and
// returns a process exit code: 0 for success, non-zero for error.
//
// This signature exists so that alternate front-end binaries (for
// example a private build that wants to share the same flag parsing)
// can delegate to Run directly instead of re-implementing main.
func Run(args []string) int {
	fs := flag.NewFlagSet("vulpineos", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage:\n")
		fmt.Fprintf(stderr, "  vulpineos [flags]\n")
		fmt.Fprintf(stderr, "  vulpineos tui [flags]\n")
		fmt.Fprintf(stderr, "  vulpineos panel [flags]\n")
		fmt.Fprintf(stderr, "  vulpineos serve [flags]\n")
		fmt.Fprintf(stderr, "  vulpineos remote [panel|tui] [flags]\n")
		fmt.Fprintf(stderr, "  vulpineos mcp [flags]\n\n")
		fmt.Fprintf(stderr, "Legacy flags remain supported.\n\n")
		fs.PrintDefaults()
	}

	var (
		binaryPath = fs.String("binary", "", "Path to VulpineOS/Camoufox binary")
		headless   = fs.Bool("headless", false, "Run in headless mode")
		profileDir = fs.String("profile", "", "Firefox profile directory")
		remoteAddr = fs.String("remote", "", "Connect to remote VulpineOS (wss://host:port/ws)")
		serve      = fs.Bool("serve", false, "Run as remote-accessible server")
		port       = fs.Int("port", 8443, "Server port (with --serve)")
		apiKey     = fs.String("api-key", "", "API key for remote authentication")
		tlsCert    = fs.String("tls-cert", "", "TLS certificate file (with --serve)")
		tlsKey     = fs.String("tls-key", "", "TLS key file (with --serve)")
		noTLS      = fs.Bool("no-tls", false, "Disable TLS (plain ws:// instead of wss://)")
		noBrowser  = fs.Bool("no-browser", false, "Start without launching browser/kernel (demo or panel-only mode)")
		mcpServer  = fs.Bool("mcp-server", false, "Run as MCP stdio server (used by OpenClaw)")
		mcpConnect = fs.String("mcp-connect", "", "WebSocket URL to connect MCP server to remote kernel")
		_          = mcpConnect // M4 remote MCP — future use
		listExt    = fs.Bool("list-extensions", false, "Print the status of optional extension providers and exit")
		showVer    = fs.Bool("version", false, "Print version and exit")
	)

	// args[0] is the program name; pass the rest to Parse.
	var parseArgs []string
	if len(args) > 1 {
		parseArgs = args[1:]
	}
	if len(parseArgs) > 0 && !strings.HasPrefix(parseArgs[0], "-") {
		return runSubcommand(args[0], parseArgs)
	}
	if err := fs.Parse(parseArgs); err != nil {
		// ContinueOnError means Parse already printed to stderr.
		return 2
	}

	if *showVer {
		fmt.Fprintf(stdout, "vulpineos %s\n", Version)
		return 0
	}

	// Initialize the extension registry. On the public build this is a
	// no-op; alternate builds may register providers via build-tagged
	// init() functions that run before this call.
	extensions.Init()

	if *listExt {
		printExtensionStatus(stdout)
		return 0
	}

	var err error
	switch {
	case *mcpServer:
		err = runMCPServer(*binaryPath, *headless, *profileDir)
	case *remoteAddr != "":
		err = runRemote(*remoteAddr, *apiKey)
	case *serve:
		err = runServe(*binaryPath, *headless, *profileDir, "0.0.0.0", *port, *apiKey, *tlsCert, *tlsKey, *noTLS, *noBrowser, false)
	default:
		err = runLocal(*binaryPath, *headless, *profileDir, *noBrowser)
	}

	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runSubcommand(program string, args []string) int {
	switch args[0] {
	case "tui":
		return runTUISubcommand(args[1:])
	case "panel":
		return runPanelSubcommand(args[1:])
	case "serve":
		return runServeSubcommand(args[1:])
	case "remote":
		return runRemoteSubcommand(args[1:])
	case "mcp":
		return runMCPSubcommand(args[1:])
	default:
		fmt.Fprintf(stderr, "error: unknown subcommand %q\n", args[0])
		return 2
	}
}

func runTUISubcommand(args []string) int {
	fs := flag.NewFlagSet("vulpineos tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	binaryPath := fs.String("binary", "", "Path to VulpineOS/Camoufox binary")
	headless := fs.Bool("headless", false, "Run in headless mode")
	profileDir := fs.String("profile", "", "Firefox profile directory")
	noBrowser := fs.Bool("no-browser", false, "Start without launching browser/kernel")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := runLocal(*binaryPath, *headless, *profileDir, *noBrowser); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runPanelSubcommand(args []string) int {
	fs := flag.NewFlagSet("vulpineos panel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	binaryPath := fs.String("binary", "", "Path to VulpineOS/Camoufox binary")
	headless := fs.Bool("headless", false, "Run in headless mode")
	profileDir := fs.String("profile", "", "Firefox profile directory")
	port := fs.Int("port", 8443, "Panel port")
	apiKey := fs.String("api-key", "", "Access key for panel auth (auto-generated if omitted)")
	noBrowser := fs.Bool("no-browser", false, "Start without launching browser/kernel")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := runServe(*binaryPath, *headless, *profileDir, "127.0.0.1", *port, *apiKey, "", "", true, *noBrowser, true); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runServeSubcommand(args []string) int {
	fs := flag.NewFlagSet("vulpineos serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	binaryPath := fs.String("binary", "", "Path to VulpineOS/Camoufox binary")
	headless := fs.Bool("headless", false, "Run in headless mode")
	profileDir := fs.String("profile", "", "Firefox profile directory")
	host := fs.String("host", "0.0.0.0", "Host/interface to bind")
	port := fs.Int("port", 8443, "Server port")
	apiKey := fs.String("api-key", "", "Access key for panel and remote auth (auto-generated if omitted)")
	tlsCert := fs.String("tls-cert", "", "TLS certificate file")
	tlsKey := fs.String("tls-key", "", "TLS key file")
	noTLS := fs.Bool("no-tls", false, "Disable TLS (plain http/ws)")
	noBrowser := fs.Bool("no-browser", false, "Start without launching browser/kernel")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := runServe(*binaryPath, *headless, *profileDir, *host, *port, *apiKey, *tlsCert, *tlsKey, *noTLS, *noBrowser, false); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runRemoteSubcommand(args []string) int {
	mode := "panel"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "panel", "tui":
			mode = args[0]
			args = args[1:]
		}
	}

	fs := flag.NewFlagSet("vulpineos remote", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "Remote panel/server URL")
	apiKey := fs.String("api-key", "", "Remote access key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *rawURL == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}

	switch mode {
	case "panel":
		if err := runRemotePanel(*rawURL, *apiKey); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return 0
	case "tui":
		wsURL, err := normalizeRemoteTUIURL(*rawURL)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if err := runRemote(wsURL, *apiKey); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "error: unknown remote mode %q (expected panel or tui)\n", mode)
		return 2
	}
}

func runMCPSubcommand(args []string) int {
	fs := flag.NewFlagSet("vulpineos mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	binaryPath := fs.String("binary", "", "Path to VulpineOS/Camoufox binary")
	headless := fs.Bool("headless", false, "Run in headless mode")
	profileDir := fs.String("profile", "", "Firefox profile directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := runMCPServer(*binaryPath, *headless, *profileDir); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func ensureAccessKey(apiKey string) (string, bool, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey != "" {
		return apiKey, false, nil
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	return hex.EncodeToString(buf), true, nil
}

func panelDisplayHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "localhost"
	default:
		return host
	}
}

func buildPanelURL(host string, port int, useTLS bool, apiKey string) string {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", panelDisplayHost(host), port),
		Path:   "/",
	}
	if strings.TrimSpace(apiKey) != "" {
		query := u.Query()
		query.Set("token", apiKey)
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func normalizeRemotePanelURL(rawURL string, apiKey string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		rawURL = "http://127.0.0.1:8443"
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported remote panel URL scheme %q", u.Scheme)
	}
	if strings.HasSuffix(u.Path, "/ws") {
		u.Path = strings.TrimSuffix(u.Path, "/ws")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	if strings.TrimSpace(apiKey) != "" {
		query := u.Query()
		query.Set("token", apiKey)
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

func normalizeRemoteTUIURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "ws://127.0.0.1:8443/ws", nil
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "ws://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported remote TUI URL scheme %q", u.Scheme)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/ws"
	} else if !strings.HasSuffix(u.Path, "/ws") {
		u.Path = strings.TrimRight(u.Path, "/") + "/ws"
	}
	return u.String(), nil
}

func printPanelAccess(host string, port int, useTLS bool, apiKey string, generated bool) string {
	panelURL := buildPanelURL(host, port, useTLS, apiKey)
	if normalized := strings.TrimSpace(host); normalized == "" || normalized == "0.0.0.0" || normalized == "::" || normalized == "[::]" {
		if normalized == "" {
			normalized = "0.0.0.0"
		}
		fmt.Fprintf(stdout, "Listening on: %s:%d\n", normalized, port)
	}
	fmt.Fprintf(stdout, "Panel URL: %s\n", panelURL)
	if generated {
		fmt.Fprintf(stdout, "API key: %s (generated)\n", apiKey)
	} else if strings.TrimSpace(apiKey) != "" {
		fmt.Fprintf(stdout, "API key: %s\n", apiKey)
	}
	return panelURL
}

func openBrowserURL(rawURL string) error {
	candidates := [][]string{
		{"open", rawURL},
		{"xdg-open", rawURL},
		{"rundll32", "url.dll,FileProtocolHandler", rawURL},
	}
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate[0]); err != nil {
			continue
		}
		cmd := exec.Command(candidate[0], candidate[1:]...)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no browser launcher available")
}

func runRemotePanel(rawURL string, apiKey string) error {
	panelURL, err := normalizeRemotePanelURL(rawURL, apiKey)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Panel URL: %s\n", panelURL)
	if strings.TrimSpace(apiKey) != "" {
		fmt.Fprintf(stdout, "API key: %s\n", apiKey)
	}
	if err := openBrowserURL(panelURL); err != nil {
		log.Printf("warning: could not open browser automatically: %v", err)
	}
	return nil
}

// runMCPServer runs as an MCP stdio server for OpenClaw integration.
// It connects to a running VulpineOS kernel and translates MCP tool calls to Juggler protocol.
func runMCPServer(binaryPath string, headless bool, profileDir string) error {
	resolvedBinaryPath, err := kernel.ResolveBinaryPath(strings.TrimSpace(binaryPath))
	if err != nil {
		return err
	}
	if warning := kernel.DetectStaleBinary(resolvedBinaryPath); warning != nil {
		log.Printf("Warning: %s", warning.Message())
	}

	k := kernel.New()
	if err := k.Start(kernel.Config{
		BinaryPath: resolvedBinaryPath,
		Headless:   headless,
		ProfileDir: profileDir,
	}); err != nil {
		return fmt.Errorf("start kernel: %w", err)
	}
	defer k.Stop()

	client := k.Client()
	extensions.InitWithClient(client)

	// Start embedded foxbridge CDP server FIRST — its Browser.enable
	// call sets up event subscriptions needed by both foxbridge and MCP.
	fb := foxbridge.New()
	if err := fb.StartEmbeddedMode(client, 9222); err != nil {
		log.Printf("foxbridge embedded mode failed: %v", err)
		// Fall back to manual Browser.enable for MCP
		if err := enableBrowser(client, "Browser.enable"); err != nil {
			return err
		}
	} else {
		defer fb.Stop()
		log.Printf("foxbridge CDP proxy on %s", fb.CDPURL())
	}

	server := mcp.NewServer(client)
	return server.Run()
}

// runLocal starts the kernel and TUI locally.
func runLocal(binaryPath string, headless bool, profileDir string, noBrowser bool) error {
	restoreLogs, _ := startLocalSessionLogging(config.Dir())
	defer restoreLogs()

	// Check if first-time setup is needed
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
		cfg = &config.Config{}
	}
	if cfg.HydrateFromOpenClawProfile() {
		if saveErr := cfg.Save(); saveErr != nil {
			log.Printf("Warning: could not persist repaired config: %v", saveErr)
		}
	}
	reconfigureRequested := config.ReconfigureRequested()
	resolvedBinaryPath := strings.TrimSpace(binaryPath)
	if resolvedBinaryPath == "" && cfg != nil {
		resolvedBinaryPath = strings.TrimSpace(cfg.BinaryPath)
	}

	if cfg.NeedsSetup() || reconfigureRequested {
		// Run setup wizard
		setupModel := setup.NewWithConfig(cfg)
		p := tea.NewProgram(setupModel, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return fmt.Errorf("setup wizard: %w", err)
		}
		finalModel := result.(setup.Model)
		if !finalModel.Done() {
			_ = config.ClearReconfigureRequest()
			return nil // user quit setup
		}
		cfg = finalModel.Config()
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		_ = config.ClearReconfigureRequest()

		// Generate OpenClaw config
		exe, _ := os.Executable()
		camoufox := resolvedBinaryPath
		if err := cfg.GenerateOpenClawConfig(exe, camoufox); err != nil {
			log.Printf("Warning: could not generate OpenClaw config: %v", err)
		}
	}

	if !noBrowser {
		var resolveErr error
		resolvedBinaryPath, resolveErr = kernel.ResolveBinaryPath(resolvedBinaryPath)
		if resolveErr != nil {
			return resolveErr
		}
	}

	// Store resolved binary path in config when available.
	if resolvedBinaryPath != "" && cfg.BinaryPath != resolvedBinaryPath {
		cfg.BinaryPath = resolvedBinaryPath
		cfg.Save()
	}

	// Always regenerate openclaw.json to ensure it matches current config
	if cfg.SetupComplete {
		exe, _ := os.Executable()
		if genErr := cfg.GenerateOpenClawConfig(exe, resolvedBinaryPath); genErr != nil {
			log.Printf("Warning: could not generate OpenClaw config: %v", genErr)
		}
	}

	var k *kernel.Kernel
	var client *juggler.Client
	var orch *orchestrator.Orchestrator
	var v *vault.DB
	var audit *runtimeaudit.Manager
	var gw *openclaw.Gateway
	var fb *foxbridge.Process
	var wd *kernel.Watchdog
	var browserEnabled bool
	var startErr error

	if !noBrowser {
		// Show loading spinner while kernel starts
		loader := loading.New("Launching VulpineOS")
		loaderProg := tea.NewProgram(loader, tea.WithAltScreen())

		go func() {
			// Open vault
			v, _ = vault.Open()
			if v != nil {
				if err := v.ReconcileNonTerminalAgents("interrupted"); err != nil {
					log.Printf("Warning: reconcile agents: %v", err)
				}
				audit = runtimeaudit.New(v)
			}

			// Start kernel
			k = kernel.New()
			startErr = k.Start(kernel.Config{
				BinaryPath: resolvedBinaryPath,
				Headless:   headless,
				ProfileDir: profileDir,
			})
			if startErr == nil {
				client = k.Client()
				if err := enableBrowser(client, "Browser.enable"); err != nil {
					startErr = err
				} else {
					browserEnabled = true
					if audit != nil {
						_, _ = audit.Log("kernel", "info", "started", "kernel started", map[string]string{
							"pid":      fmt.Sprintf("%d", k.PID()),
							"headless": fmt.Sprintf("%t", headless),
						})
						wd = kernel.NewWatchdog(k, false)
						wd.SetConfig(kernel.Config{
							BinaryPath: resolvedBinaryPath,
							Headless:   headless,
							ProfileDir: profileDir,
						})
						wd.OnEvent(func(event kernel.WatchdogEvent) {
							level := "warn"
							message := "kernel event"
							switch event.Type {
							case "crashed":
								level = "error"
								message = "kernel exited unexpectedly"
							case "restart_success":
								level = "warn"
								message = "kernel restarted"
							case "restart_failed":
								level = "error"
								message = "kernel restart failed"
							}
							metadata := map[string]string{}
							if event.Attempt > 0 {
								metadata["attempt"] = fmt.Sprintf("%d", event.Attempt)
							}
							if event.Err != nil {
								metadata["error"] = event.Err.Error()
							}
							if _, err := audit.Log("kernel", level, event.Type, message, metadata); err != nil {
								log.Printf("runtime audit kernel %s: %v", event.Type, err)
							}
						})
						wd.Start()
					}

					// Create orchestrator with optional subsystems
					if v != nil {
						model := ""
						if cfg != nil {
							model = cfg.Model
						}
						orch = orchestrator.New(k, client, v, pool.DefaultConfig(), "openclaw", orchestrator.Opts{
							AgentBus:  agentbus.New(),
							Costs:     costtrack.New(model),
							Webhooks:  webhooks.New(),
							Recording: recording.NewRecorder(),
							PageCache: pagecache.New(filepath.Join(config.Dir(), "pagecache")),
						})
						if audit != nil {
							orch.Agents.SetRuntimeAudit(audit)
						}
						orch.Start()
					}
				}
			}

			// Start foxbridge as an embedded CDP server sharing the kernel's Juggler client.
			// This avoids launching a second Firefox — OpenClaw connects to the same kernel.
			if startErr == nil && client != nil {
				fb = foxbridge.New()
				fb.SetRuntimeAudit(audit)
				fbErr := fb.StartEmbeddedMode(client, 9222)
				if fbErr != nil {
					log.Printf("embedded foxbridge not available: %v (OpenClaw will use built-in Chrome)", fbErr)
					fb = nil
				} else {
					// Set CDP URL in config so OpenClaw routes through foxbridge
					cfg.FoxbridgeCDPURL = fb.CDPURL()
					exe, _ := os.Executable()
					cfg.GenerateOpenClawConfig(exe, resolvedBinaryPath)
					log.Printf("foxbridge embedded — OpenClaw browser routed through Camoufox at %s", fb.CDPURL())
				}
			}

			// Start OpenClaw gateway for browser support
			if startErr == nil {
				gw = startGatewayIfAvailable(cfg, audit)
			}

			loaderProg.Send(loading.DoneMsg{})
		}()

		result, err := loaderProg.Run()
		if err != nil {
			return fmt.Errorf("loading screen: %w", err)
		}
		if !result.(loading.Model).Done() {
			// User quit during loading
			if k != nil {
				k.Stop()
			}
			if fb != nil {
				fb.Stop()
			}
			return nil
		}
		if startErr != nil {
			return startErr
		}
		if warning := kernel.DetectStaleBinary(resolvedBinaryPath); warning != nil {
			log.Printf("Warning: %s", warning.Message())
			if audit != nil {
				_, _ = audit.Log("kernel", "warn", "stale_binary", "selected browser binary is older than repo-local build", map[string]string{
					"selected":      warning.SelectedPath,
					"preferred":     warning.PreferredPath,
					"selected_mod":  warning.SelectedMod.UTC().Format(time.RFC3339),
					"preferred_mod": warning.PreferredMod.UTC().Format(time.RFC3339),
				})
			}
		}
		defer func() {
			if audit != nil {
				_, _ = audit.Log("kernel", "info", "stopped", "kernel stopped", map[string]string{
					"pid": fmt.Sprintf("%d", k.PID()),
				})
			}
			_ = k.Stop()
		}()
		if wd != nil {
			defer wd.Stop()
		}
		if fb != nil {
			defer fb.Stop()
		}
		if v != nil {
			defer v.Close()
		}
		if audit != nil {
			defer audit.Close()
		}
		if orch != nil {
			defer orch.Close()
		}
		if gw != nil {
			defer func() {
				if audit != nil {
					_, _ = audit.Log("gateway", "info", "stopped", "OpenClaw gateway stopped", nil)
				}
				gw.Stop()
			}()
		}
	}

	// Create the TUI after startup subsystems are fully initialized.
	app := tui.NewApp(k, client, orch, v, cfg, audit)

	if client != nil {
		// Wire the live juggler client into any build-tagged extension
		// providers. On the default public build this is a no-op
		// because privateProviders is zero-valued.
		extensions.InitWithClient(client)
		if !browserEnabled {
			if err := enableBrowser(client, "Browser.enable"); err != nil {
				return err
			}
			browserEnabled = true
		}
	}
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, runErr := p.Run()
	return runErr
}

// runRemote connects to a remote VulpineOS server and launches the TUI.
func runRemote(addr string, apiKey string) error {
	ctx := context.Background()
	rc, err := remote.Dial(ctx, addr, apiKey)
	if err != nil {
		return fmt.Errorf("connect to remote: %w", err)
	}
	defer rc.Close()

	// Create a Juggler client over the WebSocket transport
	client := juggler.NewClient(rc)
	defer client.Close()

	extensions.InitWithClient(client)
	if err := enableBrowser(client, "Browser.enable (remote)"); err != nil {
		return err
	}

	app := tui.NewApp(nil, client, nil, nil, nil, nil)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// runServe starts the kernel and exposes it via WebSocket server.
func runServe(binaryPath string, headless bool, profileDir string, host string, port int, apiKey string, tlsCert, tlsKey string, noTLS bool, noBrowser bool, openPanel bool) error {
	apiKey, generatedKey, err := ensureAccessKey(apiKey)
	if err != nil {
		return fmt.Errorf("generate access key: %w", err)
	}
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
		cfg = &config.Config{}
	}
	if cfg.HydrateFromOpenClawProfile() {
		if saveErr := cfg.Save(); saveErr != nil {
			log.Printf("Warning: could not persist repaired config: %v", saveErr)
		}
	}
	resolvedBinaryPath := strings.TrimSpace(binaryPath)
	if resolvedBinaryPath == "" && cfg != nil {
		resolvedBinaryPath = strings.TrimSpace(cfg.BinaryPath)
	}

	var (
		audit  *runtimeaudit.Manager
		k      *kernel.Kernel
		client *juggler.Client
		wd     *kernel.Watchdog
	)

	// Open vault
	v, _ := vault.Open()
	if v != nil {
		if err := v.ReconcileNonTerminalAgents("interrupted"); err != nil {
			log.Printf("Warning: reconcile agents: %v", err)
		}
		audit = runtimeaudit.New(v)
		defer v.Close()
		defer audit.Close()
	}

	if !noBrowser {
		resolvedBinaryPath, err = kernel.ResolveBinaryPath(resolvedBinaryPath)
		if err != nil {
			return err
		}
		k = kernel.New()
		if err := k.Start(kernel.Config{
			BinaryPath: resolvedBinaryPath,
			Headless:   headless,
			ProfileDir: profileDir,
		}); err != nil {
			return fmt.Errorf("start kernel: %w", err)
		}
		defer func() {
			if audit != nil {
				_, _ = audit.Log("kernel", "info", "stopped", "kernel stopped", map[string]string{
					"pid": fmt.Sprintf("%d", k.PID()),
				})
			}
			_ = k.Stop()
		}()

		client = k.Client()
		extensions.InitWithClient(client)
		if err := enableBrowser(client, "Browser.enable"); err != nil {
			return err
		}

		if audit != nil {
			_, _ = audit.Log("kernel", "info", "started", "kernel started", map[string]string{
				"pid":      fmt.Sprintf("%d", k.PID()),
				"headless": fmt.Sprintf("%t", headless),
			})
		}
		wd = kernel.NewWatchdog(k, false)
		wd.SetConfig(kernel.Config{
			BinaryPath: resolvedBinaryPath,
			Headless:   headless,
			ProfileDir: profileDir,
		})
		wd.OnEvent(func(event kernel.WatchdogEvent) {
			if audit == nil {
				return
			}
			level := "warn"
			message := "kernel event"
			switch event.Type {
			case "crashed":
				level = "error"
				message = "kernel exited unexpectedly"
			case "restart_success":
				message = "kernel restarted"
			case "restart_failed":
				level = "error"
				message = "kernel restart failed"
			}
			metadata := map[string]string{}
			if event.Attempt > 0 {
				metadata["attempt"] = fmt.Sprintf("%d", event.Attempt)
			}
			if event.Err != nil {
				metadata["error"] = event.Err.Error()
			}
			_, _ = audit.Log("kernel", level, event.Type, message, metadata)
		})
		wd.Start()
		defer wd.Stop()
		if warning := kernel.DetectStaleBinary(resolvedBinaryPath); warning != nil {
			log.Printf("Warning: %s", warning.Message())
			if audit != nil {
				_, _ = audit.Log("kernel", "warn", "stale_binary", "selected browser binary is older than repo-local build", map[string]string{
					"selected":      warning.SelectedPath,
					"preferred":     warning.PreferredPath,
					"selected_mod":  warning.SelectedMod.UTC().Format(time.RFC3339),
					"preferred_mod": warning.PreferredMod.UTC().Format(time.RFC3339),
				})
			}
		}
	} else {
		log.Printf("browser disabled — serving panel/control API without kernel")
	}

	if resolvedBinaryPath != "" && cfg != nil && cfg.BinaryPath != resolvedBinaryPath {
		cfg.BinaryPath = resolvedBinaryPath
		if err := cfg.Save(); err != nil {
			log.Printf("Warning: could not persist binary path: %v", err)
		}
	}

	// Create orchestrator with subsystems
	var orch *orchestrator.Orchestrator
	var bus *agentbus.Bus
	var costs *costtrack.Tracker
	var wh *webhooks.Manager
	var rec *recording.Recorder
	if v != nil {
		model := ""
		if cfg != nil {
			model = cfg.Model
		}
		bus = agentbus.New()
		costs = costtrack.New(model)
		wh = webhooks.New()
		rec = recording.NewRecorder()
		if k != nil && client != nil {
			orch = orchestrator.New(k, client, v, pool.DefaultConfig(), "openclaw", orchestrator.Opts{
				AgentBus:  bus,
				Costs:     costs,
				Webhooks:  wh,
				Recording: rec,
				PageCache: pagecache.New(filepath.Join(config.Dir(), "pagecache")),
			})
			if audit != nil {
				orch.Agents.SetRuntimeAudit(audit)
			}
			if startErr := orch.Start(); startErr != nil {
				log.Printf("Warning: orchestrator start: %v", startErr)
			} else {
				defer orch.Close()
			}
		}
	}

	// Create proxy rotator
	rotator := proxy.NewRotator()
	contexts := remote.NewContextRegistry()

	addr := fmt.Sprintf("%s:%d", host, port)
	server := remote.NewServer(addr, apiKey, client)

	// Wire PanelAPI for control message handling
	panelAPI := &remote.PanelAPI{
		Orchestrator: orch,
		Config:       cfg,
		Vault:        v,
		AgentBus:     bus,
		Costs:        costs,
		Webhooks:     wh,
		Recorder:     rec,
		Rotator:      rotator,
		Kernel:       k,
		Client:       client,
		Contexts:     contexts,
		RuntimeAudit: audit,
	}
	server.SetPanelAPI(panelAPI)

	// Forward telemetry events to connected clients
	if client != nil {
		for _, event := range []string{
			"Browser.telemetryUpdate",
			"Browser.injectionAttemptDetected",
			"Browser.trustWarmingStateChanged",
			"Browser.attachedToTarget",
			"Browser.detachedFromTarget",
			"Page.navigationCommitted",
			"Page.eventFired",
			"Page.frameAttached",
			"Runtime.executionContextCreated",
			"Runtime.executionContextDestroyed",
		} {
			evt := event
			client.Subscribe(evt, func(sessionID string, params json.RawMessage) {
				switch evt {
				case "Browser.attachedToTarget":
					var payload struct {
						SessionID  string `json:"sessionId"`
						TargetInfo struct {
							BrowserContextID string `json:"browserContextId"`
							URL              string `json:"url"`
						} `json:"targetInfo"`
					}
					if err := json.Unmarshal(params, &payload); err == nil {
						contexts.Attached(payload.SessionID, payload.TargetInfo.BrowserContextID, payload.TargetInfo.URL)
					}
				case "Browser.detachedFromTarget":
					var payload struct {
						SessionID string `json:"sessionId"`
					}
					if err := json.Unmarshal(params, &payload); err == nil {
						contexts.Detached(payload.SessionID)
					}
				}
				server.BroadcastEvent(evt, sessionID, params)
			})
		}
	}

	if orch != nil && v != nil {
		statusCh := orch.Agents.StatusChan()
		go func() {
			for status := range statusCh {
				if err := v.UpdateAgentStatus(status.AgentID, status.Status); err != nil {
					log.Printf("vault: update agent status %s: %v", status.AgentID, err)
				}
				if status.Tokens > 0 {
					if err := v.UpdateAgentTokens(status.AgentID, status.Tokens); err != nil {
						log.Printf("vault: update agent tokens %s: %v", status.AgentID, err)
					}
				}
				payload := map[string]interface{}{
					"agentId":   status.AgentID,
					"contextId": status.ContextID,
					"status":    status.Status,
					"objective": status.Objective,
					"tokens":    status.Tokens,
				}
				if encoded, err := json.Marshal(payload); err == nil {
					server.BroadcastEvent("Vulpine.agentStatus", "", encoded)
				}
			}
		}()

		conversationCh := orch.Agents.ConversationChan()
		go func() {
			for msg := range conversationCh {
				if err := v.AppendMessage(msg.AgentID, msg.Role, msg.Content, msg.Tokens); err != nil {
					log.Printf("vault: append message %s: %v", msg.AgentID, err)
				}
				payload := map[string]interface{}{
					"agentId": msg.AgentID,
					"role":    msg.Role,
					"content": msg.Content,
					"tokens":  msg.Tokens,
				}
				if encoded, err := json.Marshal(payload); err == nil {
					server.BroadcastEvent("Vulpine.conversation", "", encoded)
				}
			}
		}()
	}

	if audit != nil {
		sub := audit.Subscribe()
		go func() {
			for event := range sub {
				if encoded, err := json.Marshal(event); err == nil {
					server.BroadcastEvent("Vulpine.runtimeEvent", "", encoded)
				}
			}
		}()
	}

	// Serve the web panel from embedded files
	if panelFS := PanelFS(); panelFS != nil {
		remote.ServePanel(server.Mux(), panelFS)
		log.Println("web panel available at /")
	}

	if client != nil {
		// Start embedded foxbridge CDP server
		fb := foxbridge.New()
		fb.SetRuntimeAudit(audit)
		if err := fb.StartEmbeddedMode(client, 9222); err != nil {
			log.Printf("foxbridge: %v (CDP proxy not available)", err)
		} else {
			defer fb.Stop()
			cfg.FoxbridgeCDPURL = fb.CDPURL()
			exe, _ := os.Executable()
			if err := cfg.GenerateOpenClawConfig(exe, resolvedBinaryPath); err != nil {
				log.Printf("Warning: could not generate OpenClaw config: %v", err)
			} else if err := config.RepairOpenClawProfile(cfg.FoxbridgeCDPURL); err != nil {
				log.Printf("Warning: could not repair OpenClaw profile after foxbridge start: %v", err)
			}
			log.Printf("foxbridge CDP on %s", fb.CDPURL())
		}

		gw := startGatewayIfAvailable(cfg, audit)
		if gw != nil {
			panelAPI.Gateway = gw
			defer func() {
				if audit != nil {
					_, _ = audit.Log("gateway", "info", "stopped", "OpenClaw gateway stopped", nil)
				}
				gw.Stop()
			}()
		}

		log.Printf("VulpineOS kernel running (PID %d)", k.PID())
	} else {
		log.Printf("VulpineOS remote server running without kernel/browser")
	}

	panelURL := printPanelAccess(host, port, !noTLS, apiKey, generatedKey)
	if openPanel {
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := openBrowserURL(panelURL); err != nil {
				log.Printf("warning: could not open browser automatically: %v", err)
			}
		}()
	}

	if noTLS {
		log.Printf("TLS disabled — serving plain ws:// on port %d", port)
		return server.Start()
	}

	// Resolve TLS certificates
	certFile, keyFile := tlsCert, tlsKey
	if certFile == "" || keyFile == "" {
		// Auto-generate self-signed certs
		auto, autoKey, err := remote.GenerateSelfSignedCert()
		if err != nil {
			return fmt.Errorf("auto-generate TLS cert: %w", err)
		}
		certFile, keyFile = auto, autoKey
		log.Printf("Using auto-generated self-signed TLS certificate")
	}

	fp, err := remote.CertFingerprint(certFile)
	if err == nil {
		log.Printf("TLS cert fingerprint: %s", fp)
	}
	return server.StartTLS(certFile, keyFile)
}
