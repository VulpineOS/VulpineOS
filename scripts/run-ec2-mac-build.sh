#!/usr/bin/env bash
set -euo pipefail

ROOT="${VULPINE_BUILDER_ROOT:-/opt/vulpineos}"
ARTIFACTS_DIR="${ROOT}/artifacts"
LOGS_DIR="${ROOT}/logs"
RUN_PACKAGE="${VULPINE_RUN_PACKAGE:-0}"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
BUILD_LOG="${LOGS_DIR}/build-${TIMESTAMP}.log"
PACKAGE_LOG="${LOGS_DIR}/package-${TIMESTAMP}.log"
METADATA_JSON="${ARTIFACTS_DIR}/build-${TIMESTAMP}.json"
GO_BINARY="${ARTIFACTS_DIR}/vulpineos-${TIMESTAMP}"
REPO_ROOT="$(pwd)"
GIT_SHA="$(git rev-parse HEAD)"
HOSTNAME_VALUE="$(hostname)"

mkdir -p "${ARTIFACTS_DIR}" "${LOGS_DIR}"

{
  echo "timestamp=${TIMESTAMP}"
  echo "repo_root=${REPO_ROOT}"
  echo "git_sha=${GIT_SHA}"
  echo "hostname=${HOSTNAME_VALUE}"
  echo "build_log=${BUILD_LOG}"
  echo "package_log=${PACKAGE_LOG}"
  echo "go_binary=${GO_BINARY}"
} > "${LOGS_DIR}/build-${TIMESTAMP}.env"

make build 2>&1 | tee "${BUILD_LOG}"

if [[ "${RUN_PACKAGE}" == "1" ]]; then
  make package-macos 2>&1 | tee "${PACKAGE_LOG}"
fi

go build -o "${GO_BINARY}" ./cmd/vulpineos

cat > "${METADATA_JSON}" <<EOF
{
  "timestamp": "${TIMESTAMP}",
  "git_sha": "${GIT_SHA}",
  "hostname": "${HOSTNAME_VALUE}",
  "repo_root": "${REPO_ROOT}",
  "build_log": "${BUILD_LOG}",
  "package_log": "${PACKAGE_LOG}",
  "go_binary": "${GO_BINARY}",
  "run_package": ${RUN_PACKAGE}
}
EOF

echo "Build complete."
echo "Metadata: ${METADATA_JSON}"
