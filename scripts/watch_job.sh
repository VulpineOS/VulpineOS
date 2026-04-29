#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/watch_job.sh <pid> <log-path> [tail-lines]

Print a status snapshot for a long-running job tracked by exact PID and log path.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage >&2
  exit 2
fi

pid="$1"
log_path="$2"
tail_lines="${3:-40}"

if ! [[ "$pid" =~ ^[0-9]+$ ]]; then
  printf 'ERROR: pid must be numeric: %s\n' "$pid" >&2
  exit 2
fi

if ! [[ "$tail_lines" =~ ^[0-9]+$ ]] || [[ "$tail_lines" -eq 0 ]]; then
  printf 'ERROR: tail-lines must be a positive integer: %s\n' "$tail_lines" >&2
  exit 2
fi

timestamp_utc="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
if kill -0 "$pid" 2>/dev/null; then
  status="running"
else
  status="stopped"
fi

printf 'time_utc=%s\n' "$timestamp_utc"
printf 'pid=%s\n' "$pid"
printf 'status=%s\n' "$status"
printf 'log=%s\n' "$log_path"

if [[ ! -e "$log_path" ]]; then
  printf 'log_status=missing\n'
  exit 0
fi

if stat -f '%z' "$log_path" >/dev/null 2>&1; then
  log_size="$(stat -f '%z' "$log_path")"
  log_mtime="$(stat -f '%Sm' -t '%Y-%m-%dT%H:%M:%S%z' "$log_path")"
else
  log_size="$(stat -c '%s' "$log_path")"
  log_mtime="$(stat -c '%y' "$log_path")"
fi

printf 'log_status=present\n'
printf 'log_size_bytes=%s\n' "$log_size"
printf 'log_mtime=%s\n' "$log_mtime"
printf -- '--- log tail (%s lines) ---\n' "$tail_lines"
tail -n "$tail_lines" "$log_path"
