# Browser Build Tester

Tests a raw Camoufox/Firefox binary directly against the project compatibility checks. Use this to validate a binary before packaging or releasing without using the Python package.

## Prerequisites

- Python 3.9+
- Node.js (for building the TypeScript checks bundle via `esbuild`, first run only)

## Setup

```bash
# Install npm deps (once — needed to build the checks bundle)
npm install

# Install Python deps
pip install -r requirements.txt
```

## Usage

```bash
python scripts/run_tests.py <binary_path> [options]
```

**Example:**
```bash
python scripts/run_tests.py /path/to/camoufox-bin/camoufox
```

## Options

```
  binary_path           Path to the Camoufox (Firefox) binary
  --profile-count N     Number of profiles to test (1-8, default: 8)
  --secret KEY          HMAC signing key for certificate
  --save-cert PATH      Save certificate text to a file
  --no-cert             Skip certificate generation
```

## What It Tests

8 profiles total, run in two phases:

**Per-context phase (6 profiles)** — multiple macOS and Linux profiles open simultaneously in a single browser instance.

**Global phase (2 profiles)** — one macOS and one Linux profile launched with global browser configuration.

Each profile is scored across:

| Category | What it checks |
|---|---|
| Runtime compatibility | Browser automation and API compatibility |
| Engine behavior | Expected Firefox-family behavior |
| Profile consistency | Profile values remain internally consistent |
| Rendering | Graphics and media surfaces stay stable |
| Network routing | Browser networking behavior matches configuration |
| Stability | Profile state remains stable over time |
| Match Results | Configured values appear through public APIs |

## How It Differs from the Service Tests

| | Build Tester | Service Tests |
|---|---|---|
| Entry point | Raw binary path | `pip install camoufox` |
| Profile setup | Manual test harness | Via `AsyncNewContext` API |
| Global mode | Yes (`CAMOU_CONFIG` env var) | No |
| Match validation | Yes (checks injected values match page) | No |
| Proxy support | No | Yes |
| Profile count | 8 (6 per-context + 2 global) | 6 (per-context only) |

## The Checks Bundle

`scripts/checks-bundle.js` is a compiled artifact built from the TypeScript sources in `src/lib/checks/`. It is built automatically on first run. To force a rebuild, delete it:

```bash
rm scripts/checks-bundle.js
python scripts/run_tests.py <binary_path>
```

Source files:
- `src/lib/checks/index.ts` — entry point
- `src/lib/checks/core.ts` — core browser compatibility checks
- `src/lib/checks/extended.ts` — extended rendering and media checks
- `src/lib/checks/workers.ts` — worker-thread consistency checks
- `src/lib/checks/collectors.ts` — browser-state collectors
