package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"vulpineos/internal/juggler"
	"vulpineos/internal/tokenopt"
)

// tools returns the list of VulpineOS browser tools available via MCP.
func tools() []ToolDefinition {
	base := baseTools()
	base = append(base, humanTools()...)
	return append(base, extensionTools()...)
}

// baseTools returns the core browser tool definitions.
func baseTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "vulpine_navigate",
			Description: "Navigate the browser to a URL",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"url":       {Type: "string", Description: "The URL to navigate to"},
					"sessionId": {Type: "string", Description: "Target page session ID (from vulpine_new_context)"},
				},
				Required: []string{"url", "sessionId"},
			},
		},
		{
			Name:        "vulpine_snapshot",
			Description: "Get a token-optimized semantic snapshot of the page content for LLM processing. Returns compressed DOM with >50% fewer tokens than raw HTML.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId":     {Type: "string", Description: "Target page session ID"},
					"maxDepth":      {Type: "number", Description: "Max tree depth (default 10)"},
					"maxNodes":      {Type: "number", Description: "Max nodes to return (default 500)"},
					"maxTextLength": {Type: "number", Description: "Max text per node (default 200)"},
					"viewportOnly":  {Type: "boolean", Description: "Only return elements visible in the viewport (default false)"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_click",
			Description: "Click at specific coordinates on the page",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"x":         {Type: "number", Description: "X coordinate"},
					"y":         {Type: "number", Description: "Y coordinate"},
				},
				Required: []string{"sessionId", "x", "y"},
			},
		},
		{
			Name:        "vulpine_type",
			Description: "Type text into the currently focused element",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"text":      {Type: "string", Description: "Text to type"},
				},
				Required: []string{"sessionId", "text"},
			},
		},
		{
			Name:        "vulpine_screenshot",
			Description: "Take a screenshot of the current page",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_scroll",
			Description: "Scroll the page by a given amount",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"deltaY":    {Type: "number", Description: "Vertical scroll amount in pixels (positive = down)"},
				},
				Required: []string{"sessionId", "deltaY"},
			},
		},
		{
			Name:        "vulpine_new_context",
			Description: "Create a new isolated browser context with a fresh page. Returns the sessionId and contextId for subsequent operations.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
			},
		},
		{
			Name:        "vulpine_close_context",
			Description: "Close a browser context and all its pages",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"contextId": {Type: "string", Description: "Browser context ID to close"},
				},
				Required: []string{"contextId"},
			},
		},
		{
			Name:        "vulpine_get_ax_tree",
			Description: "Get the full accessibility tree of the page (injection-proof filtered)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_click_ref",
			Description: "Click an element by its ref from the optimized DOM snapshot (e.g. @0, @1). Use vulpine_snapshot first to get refs.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"ref":       {Type: "string", Description: "Element reference from snapshot (e.g. \"@0\", \"@1\")"},
				},
				Required: []string{"sessionId", "ref"},
			},
		},
		{
			Name:        "vulpine_type_ref",
			Description: "Focus an element by its ref from the optimized DOM snapshot and type text into it.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"ref":       {Type: "string", Description: "Element reference from snapshot (e.g. \"@0\", \"@1\")"},
					"text":      {Type: "string", Description: "Text to type into the element"},
				},
				Required: []string{"sessionId", "ref", "text"},
			},
		},
		{
			Name:        "vulpine_hover_ref",
			Description: "Hover over an element by its ref from the optimized DOM snapshot.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"ref":       {Type: "string", Description: "Element reference from snapshot (e.g. \"@0\", \"@1\")"},
				},
				Required: []string{"sessionId", "ref"},
			},
		},
		// --- Agent reliability tools ---
		{
			Name:        "vulpine_wait",
			Description: "Wait for a condition to be met on the page. Use this BEFORE taking actions to ensure the page is ready. Conditions: 'element' (CSS selector visible), 'text' (body contains text), 'networkIdle' (no pending requests), 'domStable' (DOM stopped changing), 'urlContains' (URL contains string).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"condition": {Type: "string", Description: "Condition type: element, text, networkIdle, domStable, urlContains"},
					"selector":  {Type: "string", Description: "CSS selector (for 'element' condition)"},
					"text":      {Type: "string", Description: "Text to match (for 'text' and 'urlContains' conditions)"},
					"timeout":   {Type: "number", Description: "Timeout in seconds (default 10, max 30)"},
				},
				Required: []string{"sessionId", "condition"},
			},
		},
		{
			Name:        "vulpine_find",
			Description: "Search for interactive elements by text content, aria-label, or placeholder. Returns matching elements with their position and role. Use this to locate elements when you don't have a ref from the snapshot.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId":  {Type: "string", Description: "Target page session ID"},
					"query":      {Type: "string", Description: "Text to search for (case-insensitive, matches text, aria-label, placeholder, title)"},
					"role":       {Type: "string", Description: "Optional: filter by element role (button, link, input, select, etc.)"},
					"maxResults": {Type: "number", Description: "Max results to return (default 5)"},
				},
				Required: []string{"sessionId", "query"},
			},
		},
		{
			Name:        "vulpine_verify",
			Description: "Verify element state after an action. Use this to confirm your action had the intended effect. Returns PASS or FAIL. Checks: 'exists', 'visible', 'checked', 'value', 'text', 'url', 'title'.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"check":     {Type: "string", Description: "What to check: exists, visible, checked, value, text, url, title"},
					"selector":  {Type: "string", Description: "CSS selector (for element checks)"},
					"expected":  {Type: "string", Description: "Expected value (for value, text, url, title checks)"},
				},
				Required: []string{"sessionId", "check"},
			},
		},
		{
			Name:        "vulpine_screenshot_diff",
			Description: "Take a screenshot checkpoint. Compares with the previous checkpoint for this session to detect if the page changed visually. Returns SAME or CHANGED. Use before and after actions to verify they had an effect.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"label":     {Type: "string", Description: "Label for this checkpoint (e.g. 'before_click', 'after_submit')"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_page_settled",
			Description: "Wait until the page is fully loaded and stable. Checks document.readyState, DOM mutations, and pending images. Use after navigation or clicking links that load new pages.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"timeout":   {Type: "number", Description: "Timeout in seconds (default 10)"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_select_option",
			Description: "Select an option from a dropdown/select element. Specify either the option value or visible text.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"selector":  {Type: "string", Description: "CSS selector for the <select> element"},
					"value":     {Type: "string", Description: "Option value to select"},
					"text":      {Type: "string", Description: "Option visible text to select (alternative to value)"},
				},
				Required: []string{"sessionId", "selector"},
			},
		},
		{
			Name:        "vulpine_fill_form",
			Description: "Fill multiple form fields at once. Pass a map of CSS selectors to values. Triggers input and change events on each field.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"fields":    {Type: "object", Description: "Map of CSS selector → value to fill"},
				},
				Required: []string{"sessionId", "fields"},
			},
		},
		{
			Name:        "vulpine_page_info",
			Description: "Get comprehensive page state: URL, title, scroll position, number of forms/inputs/buttons/links, whether you can scroll further, and whether modals are open. Use this to understand the current page before deciding what to do.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_press_key",
			Description: "Press a keyboard key or shortcut. Supports: Enter, Tab, Escape, Backspace, Delete, ArrowUp/Down/Left/Right, Home, End, PageUp/Down, Space. Modifiers: ctrl, shift, alt, meta (e.g. \"ctrl+shift\").",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"key":       {Type: "string", Description: "Key name (Enter, Tab, Escape, Backspace, ArrowDown, etc.)"},
					"modifiers": {Type: "string", Description: "Optional modifiers: ctrl, shift, alt, meta, or combinations like ctrl+shift"},
				},
				Required: []string{"sessionId", "key"},
			},
		},
		{
			Name:        "vulpine_clear_input",
			Description: "Clear the text in an input field. Optionally specify a CSS selector to focus the element first, then selects all text and deletes it.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"selector":  {Type: "string", Description: "Optional CSS selector to focus before clearing"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "vulpine_get_form_errors",
			Description: "Extract form validation error messages from the page. Checks HTML5 validation, common error CSS classes (.error, .is-invalid, [aria-invalid]), and aria-describedby messages.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"selector":  {Type: "string", Description: "CSS selector for the form (default: \"form\")"},
				},
				Required: []string{"sessionId"},
			},
		},
	}
}

// humanTools returns tool definitions for human-like interactions.
func humanTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "vulpine_human_click",
			Description: "Move mouse naturally to coordinates and click. Generates bezier curve path with overshoot and micro-jitter.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"x":         {Type: "number", Description: "X coordinate to click"},
					"y":         {Type: "number", Description: "Y coordinate to click"},
					"speed":     {Type: "string", Description: "Movement speed: slow, normal, fast (default: normal)"},
				},
				Required: []string{"sessionId", "x", "y"},
			},
		},
		{
			Name:        "vulpine_human_type",
			Description: "Type text with realistic human cadence. Variable inter-key intervals, occasional pauses.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"text":      {Type: "string", Description: "Text to type"},
					"wpm":       {Type: "number", Description: "Words per minute (default: 60)"},
				},
				Required: []string{"sessionId", "text"},
			},
		},
		{
			Name:        "vulpine_human_scroll",
			Description: "Scroll with realistic inertial decay.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"sessionId": {Type: "string", Description: "Target page session ID"},
					"deltaY":    {Type: "number", Description: "Total scroll amount in pixels (positive = down)"},
				},
				Required: []string{"sessionId", "deltaY"},
			},
		},
	}
}

// HandleToolCallDirect dispatches a tool call directly (for testing).
func HandleToolCallDirect(client *juggler.Client, name string, args json.RawMessage) (*ToolCallResult, error) {
	return HandleToolCallDirectCtx(context.Background(), client, name, args)
}

// HandleToolCallDirectCtx dispatches a tool call directly with an
// explicit context, for tests and callers that want to pass through a
// per-call deadline or sentinel value into extension handlers.
func HandleToolCallDirectCtx(ctx context.Context, client *juggler.Client, name string, args json.RawMessage) (*ToolCallResult, error) {
	tracker := NewContextTracker(client)
	return handleToolCall(ctx, client, tracker, name, args)
}

// handleToolCall dispatches a tool call to the appropriate handler.
func handleToolCall(ctx context.Context, client *juggler.Client, tracker *ContextTracker, name string, args json.RawMessage) (*ToolCallResult, error) {
	return handleToolCallFull(ctx, client, tracker, nil, name, args)
}

func handleToolCallFull(ctx context.Context, client *juggler.Client, tracker *ContextTracker, screenshots *ScreenshotTracker, name string, args json.RawMessage) (*ToolCallResult, error) {
	if res, ok := handleExtensionTool(ctx, client, name, args); ok {
		return res, nil
	}
	switch name {
	// Core browser tools
	case "vulpine_navigate":
		return handleNavigate(client, tracker, args)
	case "vulpine_snapshot":
		return handleSnapshot(client, args)
	case "vulpine_click":
		return handleClick(client, args)
	case "vulpine_type":
		return handleType(client, args)
	case "vulpine_screenshot":
		return handleScreenshot(client, args)
	case "vulpine_scroll":
		return handleScroll(client, args)
	case "vulpine_new_context":
		return handleNewContext(client, args)
	case "vulpine_close_context":
		return handleCloseContext(client, args)
	case "vulpine_get_ax_tree":
		return handleGetAXTree(client, args)
	case "vulpine_click_ref":
		return handleClickRef(client, args)
	case "vulpine_type_ref":
		return handleTypeRef(client, args)
	case "vulpine_hover_ref":
		return handleHoverRef(client, args)

	// Agent reliability tools
	case "vulpine_wait":
		return handleWait(client, args)
	case "vulpine_find":
		return handleFind(client, args)
	case "vulpine_verify":
		return handleVerify(client, args)
	case "vulpine_screenshot_diff":
		if screenshots == nil {
			screenshots = NewScreenshotTracker()
		}
		return handleScreenshotDiff(client, screenshots, args)
	case "vulpine_page_settled":
		return handlePageSettled(client, args)
	case "vulpine_select_option":
		return handleSelectOption(client, args)
	case "vulpine_fill_form":
		return handleFillForm(client, args)
	case "vulpine_page_info":
		return handleGetPageInfo(client, args)
	case "vulpine_press_key":
		return handlePressKey(client, args)
	case "vulpine_clear_input":
		return handleClearInput(client, args)
	case "vulpine_get_form_errors":
		return handleGetFormErrors(client, args)

	// Human-like interaction tools
	case "vulpine_human_click":
		return handleHumanClick(client, args)
	case "vulpine_human_type":
		return handleHumanType(client, args)
	case "vulpine_human_scroll":
		return handleHumanScroll(client, args)

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func textResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(err error) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: err.Error()}},
		IsError: true,
	}
}

// --- Tool handlers ---

func handleNavigate(client *juggler.Client, tracker *ContextTracker, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		URL       string `json:"url"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	// Resolve frame ID for this session
	ctx, err := tracker.Resolve(p.SessionID)
	if err != nil {
		return errorResult(fmt.Errorf("cannot navigate: %w", err)), nil
	}

	_, err = client.Call(p.SessionID, "Page.navigate", map[string]interface{}{
		"url":     p.URL,
		"frameId": ctx.FrameID,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Navigated to %s", p.URL)), nil
}

func handleSnapshot(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID     string `json:"sessionId"`
		MaxDepth      int    `json:"maxDepth"`
		MaxNodes      int    `json:"maxNodes"`
		MaxTextLength int    `json:"maxTextLength"`
		ViewportOnly  bool   `json:"viewportOnly"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	params := map[string]interface{}{}
	if p.MaxDepth > 0 {
		params["maxDepth"] = p.MaxDepth
	}
	if p.MaxNodes > 0 {
		params["maxNodes"] = p.MaxNodes
	}
	if p.MaxTextLength > 0 {
		params["maxTextLength"] = p.MaxTextLength
	}
	if p.ViewportOnly {
		params["viewportOnly"] = true
	}

	result, err := client.Call(p.SessionID, "Page.getOptimizedDOM", params)
	if err != nil {
		return errorResult(err), nil
	}

	// Apply viewport pruning to reduce token count when requested
	if p.ViewportOnly {
		var snapshot map[string]interface{}
		if err := json.Unmarshal(result, &snapshot); err == nil {
			if nodes, ok := snapshot["nodes"].([]interface{}); ok {
				pruner := tokenopt.NewViewportPruner(1280, 720)
				snapshot["nodes"] = pruner.Prune(nodes)
				if pruned, err := json.Marshal(snapshot); err == nil {
					return textResult(string(pruned)), nil
				}
			}
		}
		// Fall through to raw result if parsing/pruning fails
	}

	return textResult(string(result)), nil
}

func handleClick(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string  `json:"sessionId"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	// mousedown
	_, err := client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mousedown", "x": p.X, "y": p.Y,
		"button": 0, "clickCount": 1, "modifiers": 0, "buttons": 1,
	})
	if err != nil {
		return errorResult(err), nil
	}

	// mouseup
	_, err = client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseup", "x": p.X, "y": p.Y,
		"button": 0, "clickCount": 1, "modifiers": 0, "buttons": 0,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Clicked at (%v, %v)", p.X, p.Y)), nil
}

func handleType(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	_, err := client.Call(p.SessionID, "Page.insertText", map[string]interface{}{
		"text": p.Text,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Typed %d characters", len(p.Text))), nil
}

func handleScreenshot(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	result, err := client.Call(p.SessionID, "Page.screenshot", map[string]interface{}{
		"mimeType": "image/png",
		"clip":     map[string]interface{}{"x": 0, "y": 0, "width": 1280, "height": 720},
	})
	if err != nil {
		return errorResult(err), nil
	}

	var screenshot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &screenshot); err != nil {
		return errorResult(err), nil
	}

	return &ToolCallResult{
		Content: []ContentBlock{{
			Type:     "image",
			Data:     screenshot.Data,
			MimeType: "image/png",
		}},
	}, nil
}

func handleScroll(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string  `json:"sessionId"`
		DeltaY    float64 `json:"deltaY"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	_, err := client.Call(p.SessionID, "Page.dispatchWheelEvent", map[string]interface{}{
		"x": 400, "y": 300,
		"deltaX": 0, "deltaY": p.DeltaY, "deltaZ": 0,
		"modifiers": 0,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Scrolled by %v pixels", p.DeltaY)), nil
}

func handleNewContext(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	// Subscribe to get the sessionID from the attachedToTarget event
	sessionCh := make(chan string, 4)
	client.Subscribe("Browser.attachedToTarget", func(_ string, params json.RawMessage) {
		var ev struct {
			SessionID string `json:"sessionId"`
		}
		json.Unmarshal(params, &ev)
		if ev.SessionID != "" {
			select {
			case sessionCh <- ev.SessionID:
			default:
			}
		}
	})

	// Create context
	ctxResult, err := client.Call("", "Browser.createBrowserContext", map[string]interface{}{
		"removeOnDetach": true,
	})
	if err != nil {
		return errorResult(err), nil
	}

	var ctx struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(ctxResult, &ctx); err != nil {
		return errorResult(err), nil
	}

	// Create page in context
	_, err = client.Call("", "Browser.newPage", map[string]interface{}{
		"browserContextId": ctx.BrowserContextID,
	})
	if err != nil {
		return errorResult(err), nil
	}

	// Wait for session ID from event
	var sessionID string
	select {
	case sessionID = <-sessionCh:
	case <-time.After(10 * time.Second):
		return errorResult(fmt.Errorf("timed out waiting for page session")), nil
	}

	return textResult(fmt.Sprintf(`{"contextId":"%s","sessionId":"%s"}`, ctx.BrowserContextID, sessionID)), nil
}

func handleCloseContext(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		ContextID string `json:"contextId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	_, err := client.Call("", "Browser.removeBrowserContext", map[string]interface{}{
		"browserContextId": p.ContextID,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult("Context closed"), nil
}

func handleGetAXTree(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	result, err := client.Call(p.SessionID, "Accessibility.getFullAXTree", nil)
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(string(result)), nil
}

func handleClickRef(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Ref       string `json:"ref"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	// Resolve ref to coordinates
	result, err := client.Call(p.SessionID, "Page.resolveRef", map[string]interface{}{
		"ref": p.Ref,
	})
	if err != nil {
		return errorResult(err), nil
	}

	var resolved struct {
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		Found bool    `json:"found"`
	}
	if err := json.Unmarshal(result, &resolved); err != nil {
		return errorResult(err), nil
	}
	if !resolved.Found {
		return errorResult(fmt.Errorf("element ref %s not found (stale snapshot?)", p.Ref)), nil
	}

	// mousedown
	_, err = client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mousedown", "x": resolved.X, "y": resolved.Y,
		"button": 0, "clickCount": 1, "modifiers": 0, "buttons": 1,
	})
	if err != nil {
		return errorResult(err), nil
	}

	// mouseup
	_, err = client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseup", "x": resolved.X, "y": resolved.Y,
		"button": 0, "clickCount": 1, "modifiers": 0, "buttons": 0,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Clicked %s at (%v, %v)", p.Ref, resolved.X, resolved.Y)), nil
}

func handleTypeRef(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Ref       string `json:"ref"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	// Focus the element by ref
	result, err := client.Call(p.SessionID, "Page.focusByRef", map[string]interface{}{
		"ref": p.Ref,
	})
	if err != nil {
		return errorResult(err), nil
	}

	var focused struct {
		Found bool `json:"found"`
	}
	if err := json.Unmarshal(result, &focused); err != nil {
		return errorResult(err), nil
	}
	if !focused.Found {
		return errorResult(fmt.Errorf("element ref %s not found (stale snapshot?)", p.Ref)), nil
	}

	// Type the text
	_, err = client.Call(p.SessionID, "Page.insertText", map[string]interface{}{
		"text": p.Text,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Typed %d characters into %s", len(p.Text), p.Ref)), nil
}

func handleHoverRef(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Ref       string `json:"ref"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	// Resolve ref to coordinates
	result, err := client.Call(p.SessionID, "Page.resolveRef", map[string]interface{}{
		"ref": p.Ref,
	})
	if err != nil {
		return errorResult(err), nil
	}

	var resolved struct {
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		Found bool    `json:"found"`
	}
	if err := json.Unmarshal(result, &resolved); err != nil {
		return errorResult(err), nil
	}
	if !resolved.Found {
		return errorResult(fmt.Errorf("element ref %s not found (stale snapshot?)", p.Ref)), nil
	}

	// mouseMoved
	_, err = client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseMoved", "x": resolved.X, "y": resolved.Y,
		"button": 0, "clickCount": 0, "modifiers": 0, "buttons": 0,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Hovered %s at (%v, %v)", p.Ref, resolved.X, resolved.Y)), nil
}
