# Browser Service Tests

End-to-end service tests that verify a pip-installed camoufox release works correctly, including both the Firefox binary and the Python package. Network-routing cases use real proxies.

## Prerequisites

- Python 3.9+
- Node.js (for building the TypeScript checks bundle via `esbuild`)
- At least one proxy in `proxies.txt`

## Quick Start

```bash
# 1. Add your proxies (see format below)
# 2. Run the test script — it handles everything else automatically
./run_tests.sh
```

`run_tests.sh` will:
1. Install npm deps in `../build-tester/` (for `esbuild`, first run only)
2. Create a `.venv` virtualenv (first run only)
3. Install `camoufox` from the local `../pythonlib` source
4. Download the camoufox browser binary
5. Run the full test suite

## Proxies

Tests require real proxies for the network-routing cases.

Create `proxies.txt` in this directory with one proxy per line:

```
user:pass@domain:port
```

Example:
```
alice:secret123@proxy1.example.com:10000
bob:hunter2@proxy2.example.com:10000
alice:secret123@proxy1.example.com:10001
```

- Blank lines and lines starting with `#` are ignored
- Proxies are assigned round-robin across the 6 test profiles
- Fewer proxies than profiles is fine — they cycle

## Manual Setup

If you prefer to run steps individually:

```bash
# Install build-tester deps (once)
cd ../build-tester && npm install && cd ../service-tester

# Create and activate virtualenv
python3 -m venv .venv
source .venv/bin/activate

# Install camoufox from local source
pip install -e ../pythonlib

# Download the browser binary
python -m camoufox fetch

# Run tests
python run_tests.py
```

## Options

```
./run_tests.sh [options]
python run_tests.py [options]

  --browser-version VER   Camoufox version specifier (default: official/stable)
                          e.g. official/prerelease/146.0.1-beta.50
  --profile-count N       Number of profiles to test (1-6, default: 6)
  --proxies PATH          Path to proxies file (default: proxies.txt)
  --headful               Run with visible browser window
  --no-cert               Skip certificate generation
  --save-cert PATH        Save certificate text to a file
  --secret KEY            HMAC signing key for the certificate
```

## What It Tests

6 browser contexts run simultaneously across macOS and Linux-style profile configurations.

Each context is scored across these categories:

| Category | What it checks |
|---|---|
| Runtime compatibility | Browser automation and API compatibility |
| Engine behavior | Expected Firefox-family behavior |
| Profile consistency | Profile values remain internally consistent |
| Rendering | Graphics and media surfaces stay stable |
| Network routing | Network behavior matches proxy configuration |
| Stability | Profile state remains stable with other contexts open |

## Interpreting Results

| Grade | Meaning |
|---|---|
| **A** | All checks pass |
| **B** | 1–2 failures (minor) |
| **C** | 3–5 failures |
| **D** | 6–10 failures |
| **F** | 11+ failures |

A grade of **A or B** exits with code `0`. Anything worse exits with code `1`.

The cross-profile section confirms contexts remain isolated from each other during concurrent execution.

## Failure Triage

If a check fails, **fix it in the Python package** (`../pythonlib/camoufox/`), not in the test. The test is intentionally a black-box validator and only uses the public `AsyncNewContext` API.
