package remote

import (
	"encoding/json"
	"fmt"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/scripting"
	"vulpineos/internal/security"
)

type securityProtection struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Details     string `json:"details,omitempty"`
}

func (api *PanelAPI) scriptsRun(params json.RawMessage) (json.RawMessage, error) {
	if api.Client == nil {
		return nil, fmt.Errorf("juggler client not available")
	}
	var p struct {
		Script    string `json:"script"`
		ContextID string `json:"contextId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Script == "" {
		return nil, fmt.Errorf("script is required")
	}

	script, err := scripting.ParseScript([]byte(p.Script))
	if err != nil {
		return nil, err
	}

	contextID, sessionID, err := api.ensureScriptSession(p.ContextID)
	if err != nil {
		return nil, err
	}

	engine := scripting.NewEngine(api.Client)
	engine.SetSession(sessionID)
	results, runErr := engine.ExecuteWithResults(script)
	payload := map[string]interface{}{
		"ok":        runErr == nil,
		"contextId": contextID,
		"sessionId": sessionID,
		"results":   results,
		"vars":      engine.Vars(),
	}
	if runErr != nil {
		payload["error"] = runErr.Error()
	}
	return json.Marshal(payload)
}

func (api *PanelAPI) securityStatus() (json.RawMessage, error) {
	browserActive := api.Client != nil && api.Kernel != nil && api.Kernel.Running()
	securityEnabled := api.Orchestrator != nil && api.Orchestrator.SecurityEnabled
	signatureDB := security.NewSignatureDB()
	sandbox := security.NewSandbox()

	protections := []securityProtection{
		{
			Key:         "ax_filter",
			Name:        "Injection-Proof AX Filter",
			Description: "Strips hidden DOM nodes before AI-readable accessibility output.",
			Status:      ternaryStatus(browserActive, "active", "disabled"),
			Details:     "Backed by the Camoufox accessibility filter pref.",
		},
		{
			Key:         "action_lock",
			Name:        "Action-Lock",
			Description: "Freezes the page while the agent is reasoning.",
			Status:      ternaryStatus(browserActive, "active", "disabled"),
			Details:     "Backed by the patched nsDocShell suspend/resume path.",
		},
		{
			Key:         "csp",
			Name:        "CSP Header Injection",
			Description: "Injects restrictive Content-Security-Policy headers on secured contexts.",
			Status:      ternaryStatus(browserActive && securityEnabled, "active", "disabled"),
			Details:     ternaryText(securityEnabled, "Enabled for orchestrator-managed contexts.", "Security suite is not enabled for new orchestrator contexts."),
		},
		{
			Key:         "mutations",
			Name:        "DOM Mutation Monitor",
			Description: "Detects suspicious elements injected after load.",
			Status:      ternaryStatus(browserActive, "available", "disabled"),
			Details:     "Observer implementation exists, but the orchestrator does not auto-inject it yet.",
		},
		{
			Key:         "signatures",
			Name:        "Injection Signature Scanner",
			Description: "Scans page text for known prompt-injection patterns.",
			Status:      "available",
			Details:     fmt.Sprintf("%d signatures loaded; not yet wired into automatic page scans.", signatureDB.Count()),
		},
		{
			Key:         "sandbox",
			Name:        "Sandboxed JS Evaluation",
			Description: "Wraps JS evaluation with blocked network-capable APIs.",
			Status:      "available",
			Details:     fmt.Sprintf("Blocked APIs: %s.", joinStrings(sandbox.BlockedAPIs(), ", ")),
		},
		{
			Key:         "optimized_dom",
			Name:        "Token-Optimized DOM",
			Description: "Compressed DOM export optimized for model context windows.",
			Status:      ternaryStatus(browserActive, "active", "disabled"),
			Details:     "Available through the browser protocol and MCP toolchain.",
		},
	}

	return json.Marshal(map[string]interface{}{
		"browserActive":         browserActive,
		"securityEnabled":       securityEnabled,
		"signaturePatternCount": signatureDB.Count(),
		"sandboxBlockedAPIs":    sandbox.BlockedAPIs(),
		"protections":           protections,
	})
}

func (api *PanelAPI) ensureScriptSession(contextID string) (string, string, error) {
	if api.Client == nil {
		return "", "", fmt.Errorf("juggler client not available")
	}
	if api.Contexts != nil {
		if contextID == "" {
			if contexts := api.Contexts.List(); len(contexts) > 0 {
				contextID = contexts[0].ID
			}
		}
		if contextID != "" {
			if sessionID := api.Contexts.SessionForContext(contextID); sessionID != "" {
				return contextID, sessionID, nil
			}
		}
	}

	if contextID == "" {
		result, err := api.Client.Call("", "Browser.createBrowserContext", map[string]interface{}{"removeOnDetach": false})
		if err != nil {
			return "", "", err
		}
		var created juggler.CreateBrowserContextResult
		if err := json.Unmarshal(result, &created); err != nil {
			return "", "", fmt.Errorf("parse createBrowserContext result: %w", err)
		}
		contextID = created.BrowserContextID
		if api.Contexts != nil {
			api.Contexts.Created(contextID)
		}
	}

	sessionCh := make(chan string, 4)
	if api.Client != nil {
		api.Client.Subscribe("Browser.attachedToTarget", func(_ string, params json.RawMessage) {
			var ev juggler.AttachedToTarget
			if err := json.Unmarshal(params, &ev); err == nil && ev.TargetInfo.BrowserContextID == contextID && ev.SessionID != "" {
				select {
				case sessionCh <- ev.SessionID:
				default:
				}
			}
		})
	}

	if _, err := api.Client.Call("", "Browser.newPage", map[string]interface{}{"browserContextId": contextID}); err != nil {
		return "", "", err
	}

	sessionID, err := api.waitForContextSession(contextID, sessionCh, 10*time.Second)
	if err != nil {
		return "", "", err
	}
	return contextID, sessionID, nil
}

func (api *PanelAPI) waitForContextSession(contextID string, sessionCh <-chan string, timeout time.Duration) (string, error) {
	if api.Contexts != nil {
		if sessionID := api.Contexts.SessionForContext(contextID); sessionID != "" {
			return sessionID, nil
		}
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if api.Contexts != nil {
			if sessionID := api.Contexts.SessionForContext(contextID); sessionID != "" {
				return sessionID, nil
			}
		}
		select {
		case sessionID := <-sessionCh:
			return sessionID, nil
		case <-ticker.C:
		case <-deadline.C:
			return "", fmt.Errorf("timed out waiting for page session")
		}
	}
}

func ternaryStatus(cond bool, yes string, no string) string {
	if cond {
		return yes
	}
	return no
}

func ternaryText(cond bool, yes string, no string) string {
	if cond {
		return yes
	}
	return no
}

func joinStrings(values []string, sep string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for i := 1; i < len(values); i++ {
		out += sep + values[i]
	}
	return out
}
