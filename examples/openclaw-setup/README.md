# OpenClaw Setup Example

This example shows the two supported ways to run OpenClaw with
VulpineOS:

1. MCP-first: OpenClaw talks to `vulpineos --mcp-server`
2. Direct CDP: OpenClaw talks to foxbridge / embedded foxbridge through
   `browser.cdpUrl`

Use this as a minimal starting point for local development or hosted
worker setup.

## Prerequisites

- `vulpineos` built from this repo
- OpenClaw installed (`npm install` in the VulpineOS repo or a global
  install)
- a configured model provider in your OpenClaw config or environment
- a Camoufox binary if you want browser-backed runs

## Option A: MCP-first

The MCP path disables OpenClaw's built-in Chromium browser and routes
browser operations through the VulpineOS MCP server.

Copy [openclaw.mcp.json](openclaw.mcp.json) into an OpenClaw profile as
`openclaw.json`, then adjust the `command` path if needed.

```bash
mkdir -p ~/.openclaw-vulpine
cp examples/openclaw-setup/openclaw.mcp.json ~/.openclaw-vulpine/openclaw.json
openclaw --profile vulpine
```

This is the best default when you want the VulpineOS MCP tool surface
instead of raw CDP.

## Option B: Direct CDP through foxbridge

The CDP path keeps OpenClaw's browser enabled and points it at a local
foxbridge or embedded-foxbridge endpoint.

Copy [openclaw.cdp.json](openclaw.cdp.json) into an OpenClaw profile and
update `browser.cdpUrl` if your foxbridge endpoint is different.

```bash
mkdir -p ~/.openclaw-vulpine
cp examples/openclaw-setup/openclaw.cdp.json ~/.openclaw-vulpine/openclaw.json
openclaw --profile vulpine
```

Use this when you want Chrome-style CDP behavior through foxbridge.

## VulpineOS-generated config behavior

VulpineOS can also generate its own `openclaw.json`.

There are two relevant runtime shapes in this repo:

- `internal/openclaw/config.go`:
  generates an MCP-first config with `browser.enabled = false`
- `internal/config/config.go`:
  generates the runtime Vulpine profile config and sets `browser.cdpUrl`
  only when foxbridge is available; otherwise OpenClaw falls back to its
  built-in Chromium

That means the "foxbridge fallback" story is:

- if foxbridge is running, OpenClaw can be pointed at Camoufox through
  `cdpUrl`
- if foxbridge is not available and you are using the runtime config
  generator, OpenClaw can still run with its own Chromium
- if you choose the MCP-first example, the browser stays disabled and
  VulpineOS owns browser operations through MCP

## Simple task flow

Use [task.txt](task.txt) as a minimal first run prompt:

```bash
openclaw --profile vulpine "$(cat examples/openclaw-setup/task.txt)"
```

The task is intentionally simple so you can confirm:

1. config loads
2. provider/auth is working
3. OpenClaw can talk to VulpineOS through the selected path

## Files in this example

- [openclaw.mcp.json](openclaw.mcp.json): MCP-first config
- [openclaw.cdp.json](openclaw.cdp.json): direct CDP/foxbridge config
- [task.txt](task.txt): small smoke-test task
