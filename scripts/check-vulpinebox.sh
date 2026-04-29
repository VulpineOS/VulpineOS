#!/usr/bin/env bash
set -euo pipefail

failures=0

pass() {
  printf 'OK: %s\n' "$1"
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    pass "Docker daemon is reachable"
  else
    fail "Docker is installed but the daemon is not reachable"
  fi
else
  fail "Docker CLI is not installed"
fi

if [ -n "${VULPINE_API_KEY:-}" ]; then
  pass "VULPINE_API_KEY is set"
else
  fail "VULPINE_API_KEY is not set"
fi

if [ -x "dist/camoufox-linux/camoufox" ]; then
  pass "Linux browser artifact exists at dist/camoufox-linux/camoufox"
elif [ -e "dist/camoufox-linux/camoufox" ]; then
  fail "dist/camoufox-linux/camoufox exists but is not executable"
else
  fail "Linux browser artifact missing at dist/camoufox-linux/camoufox"
fi

if [ -f "Dockerfile.vulpinebox" ]; then
  pass "Dockerfile.vulpinebox exists"
else
  fail "Dockerfile.vulpinebox is missing"
fi

if [ -f "docker-compose.yml" ]; then
  pass "docker-compose.yml exists"
else
  fail "docker-compose.yml is missing"
fi

if [ "$failures" -ne 0 ]; then
  cat >&2 <<'EOF'

Vulpine-Box preflight failed.

Expected setup:
  export VULPINE_API_KEY=$(openssl rand -hex 32)
  # place the Linux Camoufox artifact at:
  # dist/camoufox-linux/camoufox
  # start Docker Desktop or your Docker daemon

Then rerun:
  ./scripts/check-vulpinebox.sh
  docker compose up -d
EOF
  exit 1
fi

printf 'Vulpine-Box preflight passed. Run: docker compose up -d\n'
