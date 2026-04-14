package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"path/filepath"

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
		noBrowser  = fs.Bool("no-browser", false, "Start TUI without launching browser (demo mode)")
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
		err = runServe(*binaryPath, *headless, *profileDir, *port, *apiKey, *tlsCert, *tlsKey, *noTLS)
	default:
		err = runLocal(*binaryPath, *headless, *profileDir, *noBrowser)
	}

	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// runMCPServer runs as an MCP stdio server for OpenClaw integration.
// It connects to a running VulpineOS kernel and translates MCP tool calls to Juggler protocol.
func runMCPServer(binaryPath string, headless bool, profileDir string) error {
	k := kernel.New()
	if err := k.Start(kernel.Config{
		BinaryPath: binaryPath,
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
	// Check if first-time setup is needed
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
		cfg = &config.Config{}
	}

	if cfg.NeedsSetup() {
		// Run setup wizard
		setupModel := setup.New()
		p := tea.NewProgram(setupModel, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return fmt.Errorf("setup wizard: %w", err)
		}
		finalModel := result.(setup.Model)
		if !finalModel.Done() {
			return nil // user quit setup
		}
		cfg = finalModel.Config()
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		// Generate OpenClaw config
		exe, _ := os.Executable()
		camoufox := binaryPath
		if err := cfg.GenerateOpenClawConfig(exe, camoufox); err != nil {
			log.Printf("Warning: could not generate OpenClaw config: %v", err)
		}
	}

	// Store binary path in config if provided
	if binaryPath != "" && cfg.BinaryPath != binaryPath {
		cfg.BinaryPath = binaryPath
		cfg.Save()
	}

	// Always regenerate openclaw.json to ensure it matches current config
	if cfg.SetupComplete {
		exe, _ := os.Executable()
		if genErr := cfg.GenerateOpenClawConfig(exe, cfg.BinaryPath); genErr != nil {
			log.Printf("Warning: could not generate OpenClaw config: %v", genErr)
		}
	}

	var k *kernel.Kernel
	var client *juggler.Client
	var orch *orchestrator.Orchestrator
	var v *vault.DB
	var gw *openclaw.Gateway
	var fb *foxbridge.Process
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
			}

			// Start kernel
			k = kernel.New()
			startErr = k.Start(kernel.Config{
				BinaryPath: binaryPath,
				Headless:   headless,
				ProfileDir: profileDir,
			})
			if startErr == nil {
				client = k.Client()

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
					orch.Start()
				}
			}

			// Start foxbridge as an embedded CDP server sharing the kernel's Juggler client.
			// This avoids launching a second Firefox — OpenClaw connects to the same kernel.
			if client != nil {
				fb = foxbridge.New()
				fbErr := fb.StartEmbeddedMode(client, 9222)
				if fbErr != nil {
					log.Printf("embedded foxbridge not available: %v (OpenClaw will use built-in Chrome)", fbErr)
					fb = nil
				} else {
					// Set CDP URL in config so OpenClaw routes through foxbridge
					cfg.FoxbridgeCDPURL = fb.CDPURL()
					exe, _ := os.Executable()
					cfg.GenerateOpenClawConfig(exe, binaryPath)
					log.Printf("foxbridge embedded — OpenClaw browser routed through Camoufox at %s", fb.CDPURL())
				}
			}

			// Start OpenClaw gateway for browser support
			mgr := openclaw.NewManager("")
			if mgr.OpenClawInstalled() {
				gw = openclaw.NewGateway("")
				if gwErr := gw.Start(); gwErr != nil {
					log.Printf("Warning: OpenClaw gateway failed to start: %v (browser tools won't work)", gwErr)
				}
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
			return fmt.Errorf("start kernel: %w", startErr)
		}
		defer k.Stop()
		if fb != nil {
			defer fb.Stop()
		}
		if v != nil {
			defer v.Close()
		}
		if orch != nil {
			defer orch.Close()
		}
		if gw != nil {
			defer gw.Stop()
		}
	}

	// Create TUI with event subscriptions in place before Browser.enable
	app := tui.NewApp(k, client, orch, v, cfg)

	if client != nil {
		// Wire the live juggler client into any build-tagged extension
		// providers. On the default public build this is a no-op
		// because privateProviders is zero-valued.
		extensions.InitWithClient(client)
		if err := enableBrowser(client, "Browser.enable"); err != nil {
			return err
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

	app := tui.NewApp(nil, client, nil, nil, nil)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// runServe starts the kernel and exposes it via WebSocket server.
func runServe(binaryPath string, headless bool, profileDir string, port int, apiKey string, tlsCert, tlsKey string, noTLS bool) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
		cfg = &config.Config{}
	}

	k := kernel.New()
	if err := k.Start(kernel.Config{
		BinaryPath: binaryPath,
		Headless:   headless,
		ProfileDir: profileDir,
	}); err != nil {
		return fmt.Errorf("start kernel: %w", err)
	}
	defer k.Stop()

	client := k.Client()
	extensions.InitWithClient(client)
	if err := enableBrowser(client, "Browser.enable"); err != nil {
		return err
	}

	// Open vault
	v, _ := vault.Open()
	if v != nil {
		if err := v.ReconcileNonTerminalAgents("interrupted"); err != nil {
			log.Printf("Warning: reconcile agents: %v", err)
		}
		defer v.Close()
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
		orch = orchestrator.New(k, client, v, pool.DefaultConfig(), "openclaw", orchestrator.Opts{
			AgentBus:  bus,
			Costs:     costs,
			Webhooks:  wh,
			Recording: rec,
			PageCache: pagecache.New(filepath.Join(config.Dir(), "pagecache")),
		})
		if startErr := orch.Start(); startErr != nil {
			log.Printf("Warning: orchestrator start: %v", startErr)
		} else {
			defer orch.Close()
		}
	}

	// Create proxy rotator
	rotator := proxy.NewRotator()
	contexts := remote.NewContextRegistry()

	addr := fmt.Sprintf(":%d", port)
	server := remote.NewServer(addr, apiKey, client)

	// Wire PanelAPI for control message handling
	server.SetPanelAPI(&remote.PanelAPI{
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
	})

	// Forward telemetry events to connected clients
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

	// Serve the web panel from embedded files
	if panelFS := PanelFS(); panelFS != nil {
		remote.ServePanel(server.Mux(), panelFS)
		log.Println("web panel available at /")
	}

	// Start embedded foxbridge CDP server
	fb := foxbridge.New()
	if err := fb.StartEmbeddedMode(client, 9222); err != nil {
		log.Printf("foxbridge: %v (CDP proxy not available)", err)
	} else {
		defer fb.Stop()
		log.Printf("foxbridge CDP on %s", fb.CDPURL())
	}

	log.Printf("VulpineOS kernel running (PID %d)", k.PID())

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
