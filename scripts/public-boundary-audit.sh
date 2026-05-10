#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
workspace_root="$(cd "${repo_root}/.." && pwd)"

public_repo_names=("$(basename "${repo_root}")")
public_repo_paths=("${repo_root}")
repo_list_file="${VULPINE_PUBLIC_AUDIT_REPOS:-${repo_root}/.public-boundary-repos.local}"
if [[ -f "${repo_list_file}" ]]; then
  public_repo_names=()
  public_repo_paths=()
  while IFS= read -r repo_entry || [[ -n "${repo_entry}" ]]; do
    [[ -z "${repo_entry}" || "${repo_entry}" =~ ^[[:space:]]*# ]] && continue
    if [[ "${repo_entry}" = /* ]]; then
      repo_path="${repo_entry}"
    else
      repo_path="${workspace_root}/${repo_entry}"
    fi
    public_repo_names+=("$(basename "${repo_path}")")
    public_repo_paths+=("${repo_path}")
  done < "${repo_list_file}"
fi

exclude_specs=(
  ":(exclude)go.sum"
  ":(glob,exclude)**/node_modules/**"
  ":(glob,exclude)**/dist/**"
  ":(glob,exclude)**/build/**"
  ":(glob,exclude)**/.next/**"
  ":(glob,exclude)**/coverage/**"
  ":(glob,exclude)**/.turbo/**"
  ":(glob,exclude)**/public/llms.txt"
  ":(glob,exclude)**/public/llms-full.txt"
  ":(glob,exclude)**/docs/public/llms.txt"
  ":(glob,exclude)**/docs/public/llms-full.txt"
)

findings=0
local_denylist_file="${VULPINE_PUBLIC_AUDIT_DENYLIST:-${repo_root}/.public-boundary-denylist.local}"
local_denylist_descriptions=()
local_denylist_patterns=()

fail() {
  findings=$((findings + 1))
  printf 'FAIL: %s\n' "$1"
}

info() {
  printf 'INFO: %s\n' "$1"
}

load_local_denylist() {
  local line description pattern

  [[ -f "$local_denylist_file" ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "${line//[[:space:]]/}" ]] || continue
    [[ "$line" =~ ^[[:space:]]*# ]] && continue

    if [[ "$line" == *$'\t'* ]]; then
      description="${line%%$'\t'*}"
      pattern="${line#*$'\t'}"
    else
      description="local denylist pattern"
      pattern="$line"
    fi

    [[ -n "$pattern" ]] || continue
    local_denylist_descriptions+=("$description")
    local_denylist_patterns+=("$pattern")
  done < "$local_denylist_file"
}

check_origin_remote() {
  local repo="$1"
  local expected_name="$2"
  local origin_url

  origin_url="$(git -C "$repo" remote get-url origin 2>/dev/null || true)"
  if [[ -z "$origin_url" ]]; then
    fail "${expected_name}: missing origin remote"
    return
  fi

  if [[ ! "$origin_url" =~ ^(https://github\.com/|git@github\.com:)VulpineOS/${expected_name}(\.git)?$ ]]; then
    fail "${expected_name}: origin remote is not VulpineOS/${expected_name} (${origin_url})"
  fi
}

check_upstream_push_blocked() {
  local repo="$1"
  local expected_name="$2"
  local upstream_url
  local upstream_pushurl

  upstream_url="$(git -C "$repo" remote get-url upstream 2>/dev/null || true)"
  [[ -n "$upstream_url" ]] || return 0

  upstream_pushurl="$(git -C "$repo" config --get remote.upstream.pushurl || true)"
  if [[ "$upstream_pushurl" != "DISABLED" ]]; then
    fail "${expected_name}: upstream pushurl must be DISABLED (found: ${upstream_pushurl:-<unset>})"
  fi
}

scan_files() {
  local repo="$1"
  shift
  local pattern_args=("$@")
  git -C "$repo" grep -nI --color=never --perl-regexp "${pattern_args[@]}" -- . "${exclude_specs[@]}"
}

check_pattern() {
  local repo="$1"
  local expected_name="$2"
  local description="$3"
  local pattern="$4"
  local matches

  matches="$(scan_files "$repo" -e "$pattern" || true)"
  if [[ -n "$matches" ]]; then
    fail "${expected_name}: ${description}"
    printf '%s\n' "$matches"
  fi
}

check_vulpineos_public_polish() {
  local repo="$1"
  local matches

  matches="$(git -C "$repo" grep -nI --color=never --perl-regexp '[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]' -- web/src || true)"
  if [[ -n "$matches" ]]; then
    fail "VulpineOS: web panel source contains decorative emoji"
    printf '%s\n' "$matches"
  fi

  matches="$(git -C "$repo" grep -nI --color=never --perl-regexp '\b(window\.)?(alert|confirm)\s*\(' -- web/src || true)"
  if [[ -n "$matches" ]]; then
    fail "VulpineOS: web panel source uses browser-native alert/confirm dialogs"
    printf '%s\n' "$matches"
  fi

  extra_media="$(
    git -C "$repo" ls-files 'assets/*.gif' 'assets/*.jpeg' 'assets/*.jpg' 'assets/*.png' 'assets/*.webp' \
      | grep -Ev '^assets/(VulpineOSBanner\.png|VulpineOSCircleLogo\.png|VulpineOSLogo\.png)$' || true
  )"
  if [[ -n "$extra_media" ]]; then
    fail "VulpineOS: unreviewed tracked screenshot/media assets"
    printf '%s\n' "$extra_media"
  fi
}

check_repo() {
  local repo="$1"
  local expected_name="$2"

  if [[ ! -d "$repo/.git" ]]; then
    fail "${expected_name}: repo not found at ${repo}"
    return
  fi

  info "Auditing ${expected_name}"
  check_origin_remote "$repo" "$expected_name"
  check_upstream_push_blocked "$repo" "$expected_name"

  for i in "${!local_denylist_patterns[@]}"; do
    check_pattern "$repo" "$expected_name" "tracked local denylist match (${local_denylist_descriptions[$i]})" "${local_denylist_patterns[$i]}"
  done

  check_pattern "$repo" "$expected_name" "tracked macOS absolute path" '(^|[^A-Za-z0-9_])/Users/(?!<user>|<username>|example/|name/|runner/)[A-Za-z0-9._-]+/'
  check_pattern "$repo" "$expected_name" "tracked Linux absolute path" '(^|[^A-Za-z0-9_])/home/(?!<user>|<username>|example/|name/|appveyor/|runner/|runneradmin/|ubuntu/|vsts/)[A-Za-z0-9._-]+/'
  check_pattern "$repo" "$expected_name" "tracked Windows absolute path" '(^|[^A-Za-z0-9_])[A-Za-z]:\\\\Users\\\\(?!<user>|<username>|example\\\\|name\\\\)[^\\\\\\s]+\\\\'
  check_pattern "$repo" "$expected_name" "high-confidence secret token" 'ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|lin_api_[A-Za-z0-9]{20,}|xox[pbar]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj-)?[A-Za-z0-9]{20,}'

  if [[ "$expected_name" == "VulpineOS" ]]; then
    check_vulpineos_public_polish "$repo"
  fi
}

load_local_denylist

for i in "${!public_repo_paths[@]}"; do
  check_repo "${public_repo_paths[$i]}" "${public_repo_names[$i]}"
done

if (( findings > 0 )); then
  printf '\nBoundary audit failed with %d finding(s).\n' "$findings"
  exit 1
fi

printf '\nBoundary audit passed.\n'
