#!/usr/bin/env bash
set -euo pipefail

ROOT="${VULPINE_BUILDER_ROOT:-/opt/vulpineos}"
ARTIFACTS_DIR="${ROOT}/artifacts"
LOGS_DIR="${ROOT}/logs"
SRC_DIR="${ROOT}/src"

mkdir -p "${ARTIFACTS_DIR}" "${LOGS_DIR}" "${SRC_DIR}"

if ! xcode-select -p >/dev/null 2>&1; then
  echo "Xcode Command Line Tools are not installed. Run 'xcode-select --install' and rerun."
  exit 1
fi

if command -v xcodebuild >/dev/null 2>&1; then
  sudo xcodebuild -license accept >/dev/null 2>&1 || true
fi

if ! command -v brew >/dev/null 2>&1; then
  NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

eval "$(/opt/homebrew/bin/brew shellenv 2>/dev/null || /usr/local/bin/brew shellenv)"

brew update
brew install git gh jq node@22 python@3.12 sccache watchman

if ! grep -q 'node@22' <<<"${PATH}"; then
  echo "Add Homebrew node@22 to PATH if needed:"
  echo "  export PATH=\"$(brew --prefix node@22)/bin:\$PATH\""
fi

echo "Builder bootstrap complete."
echo "Root: ${ROOT}"
echo "Artifacts: ${ARTIFACTS_DIR}"
echo "Logs: ${LOGS_DIR}"
