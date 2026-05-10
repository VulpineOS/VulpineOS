#!/usr/bin/env python3

from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
WORKSPACE_ROOT = REPO_ROOT.parent

def configured_public_repos() -> list[tuple[str, Path]]:
    repo_list = Path(os.environ.get("VULPINE_PUBLIC_AUDIT_REPOS", REPO_ROOT / ".public-boundary-repos.local"))
    if not repo_list.exists():
        return [(REPO_ROOT.name, REPO_ROOT)]

    repos: list[tuple[str, Path]] = []
    for raw_line in repo_list.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        path = Path(line) if line.startswith("/") else WORKSPACE_ROOT / line
        repos.append((path.name, path))
    return repos or [(REPO_ROOT.name, REPO_ROOT)]


PUBLIC_REPOS = configured_public_repos()

EXCLUDE_SPECS = [
    ":(exclude)scripts/public-boundary-audit.sh",
    ":(exclude)scripts/public-history-audit.py",
    ":(glob,exclude)**/node_modules/**",
    ":(glob,exclude)**/dist/**",
    ":(glob,exclude)**/build/**",
    ":(glob,exclude)**/.next/**",
    ":(glob,exclude)**/coverage/**",
    ":(glob,exclude)**/.turbo/**",
    ":(glob,exclude)**/public/llms.txt",
    ":(glob,exclude)**/public/llms-full.txt",
    ":(glob,exclude)**/docs/public/llms.txt",
    ":(glob,exclude)**/docs/public/llms-full.txt",
]

PUBLIC_REVSET = ["HEAD", "--remotes=origin", "--tags"]
LOCAL_DENYLIST_FILE = Path(
    os.environ.get("VULPINE_PUBLIC_AUDIT_DENYLIST", REPO_ROOT / ".public-boundary-denylist.local")
)

MESSAGE_PATTERNS = [
    (
        "high-confidence secret token",
        r"ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|lin_api_[A-Za-z0-9]{20,}|xox[pbar]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj-)?[A-Za-z0-9]{20,}",
    ),
]

DIFF_PATTERNS = [
    (
        "macOS absolute path",
        r"/Users/[A-Za-z0-9._-]+/",
        r"(^|[^A-Za-z0-9_])/Users/(?!<user>|<username>|example/|name/|runner/)[A-Za-z0-9._-]+/",
    ),
    (
        "Linux absolute path",
        r"/home/[A-Za-z0-9._-]+/",
        r"(^|[^A-Za-z0-9_])/home/(?!<user>|<username>|example/|name/|appveyor/|runner/|runneradmin/|ubuntu/|vsts/)[A-Za-z0-9._-]+/",
    ),
    (
        "Windows absolute path",
        r"[A-Za-z]:\\\\Users\\\\[^\\\\]+\\\\",
        r"(^|[^A-Za-z0-9_])[A-Za-z]:\\\\Users\\\\(?!<user>|<username>|example\\\\|name\\\\)[^\\\\\\s]+\\\\",
    ),
    (
        "high-confidence secret token",
        r"ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|lin_api_[A-Za-z0-9]{20,}|xox[pbar]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj-)?[A-Za-z0-9]{20,}",
        r"ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|lin_api_[A-Za-z0-9]{20,}|xox[pbar]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj-)?[A-Za-z0-9]{20,}",
    ),
]


def load_local_denylist() -> list[tuple[str, str]]:
    if not LOCAL_DENYLIST_FILE.exists():
        return []

    patterns: list[tuple[str, str]] = []
    for raw_line in LOCAL_DENYLIST_FILE.read_text().splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if "\t" in raw_line:
            description, pattern = raw_line.split("\t", 1)
            description = description.strip()
            pattern = pattern.strip()
        else:
            description = "local denylist pattern"
            pattern = line
        if pattern:
            patterns.append((description or "local denylist pattern", pattern))
    return patterns


def run(repo: Path, args: list[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(repo), *args],
        text=True,
        capture_output=True,
        check=False,
    )


def print_fail(message: str, details: str | None = None) -> None:
    print(f"FAIL: {message}")
    if details:
        print(details.rstrip())


def compile_pattern(name: str, description: str, pattern: str) -> re.Pattern[str] | None:
    try:
        return re.compile(pattern, re.MULTILINE)
    except re.error as err:
        print_fail(f"{name}: invalid audit regex for {description}", str(err))
        return None


def audit_commit_messages(name: str, repo: Path, patterns: list[tuple[str, str]]) -> int:
    proc = run(repo, ["log", *PUBLIC_REVSET, "--format=%H%x00%B%x00==END=="])
    if proc.returncode != 0:
        print_fail(f"{name}: unable to read commit history", proc.stderr)
        return 1

    failures = 0
    text = proc.stdout
    for description, pattern in patterns:
        regex = compile_pattern(name, description, pattern)
        if regex is None:
            failures += 1
            continue
        match = regex.search(text)
        if not match:
            continue
        prefix = text[: match.start()]
        parts = prefix.split("\x00")
        commit = parts[-2].strip() if len(parts) >= 2 else "<unknown>"
        snippet = text[match.start() : match.start() + 160].splitlines()[0]
        print_fail(f"{name}: commit message matched {description} in {commit}", snippet)
        failures += 1
    return failures


def audit_path_history(name: str, repo: Path, patterns: list[tuple[str, str]]) -> int:
    if not patterns:
        return 0

    proc = run(repo, ["log", *PUBLIC_REVSET, "--name-only", "--format=commit:%H"])
    if proc.returncode != 0:
        print_fail(f"{name}: unable to read path history", proc.stderr)
        return 1

    failures = 0
    compiled_patterns: list[tuple[str, re.Pattern[str]]] = []
    for description, pattern in patterns:
        regex = compile_pattern(name, description, pattern)
        if regex is None:
            failures += 1
            continue
        compiled_patterns.append((description, regex))

    lines = proc.stdout.splitlines()
    current_commit = "<unknown>"
    for line in lines:
        if line.startswith("commit:"):
            current_commit = line.split(":", 1)[1]
            continue
        if not line:
            continue
        for description, regex in compiled_patterns:
            if regex.search(line):
                print_fail(f"{name}: path history matched {description} in {current_commit}", line)
                failures += 1
                break
    return failures


def audit_diff_history(name: str, repo: Path, patterns: list[tuple[str, str, str]]) -> int:
    failures = 0
    for description, pickaxe_pattern, strict_pattern in patterns:
        args = [
            "log",
            *PUBLIC_REVSET,
            "--pickaxe-regex",
            "-S",
            pickaxe_pattern,
            "--format=%H",
            "--",
            ".",
            *EXCLUDE_SPECS,
        ]
        proc = run(repo, args)
        if proc.returncode not in (0, 1):
            print_fail(f"{name}: unable to scan diff history for {description}", proc.stderr)
            failures += 1
            continue
        commits = [line.strip() for line in proc.stdout.splitlines() if line.strip()]
        if not commits:
            continue
        strict_regex = compile_pattern(name, description, strict_pattern)
        if strict_regex is None:
            failures += 1
            continue
        for commit in commits:
            show_args = [
                "show",
                "--format=",
                commit,
                "--",
                ".",
                *EXCLUDE_SPECS,
            ]
            show_proc = run(repo, show_args)
            if show_proc.returncode != 0:
                print_fail(f"{name}: unable to inspect commit {commit} for {description}", show_proc.stderr)
                failures += 1
                break
            if not strict_regex.search(show_proc.stdout):
                continue
            print_fail(f"{name}: diff history matched {description}", commit)
            failures += 1
            break
    return failures


def audit_local_diff_history(name: str, repo: Path, patterns: list[tuple[str, str]]) -> int:
    if not patterns:
        return 0

    compiled_patterns: list[tuple[str, re.Pattern[str]]] = []
    failures = 0
    for description, pattern in patterns:
        regex = compile_pattern(name, description, pattern)
        if regex is None:
            failures += 1
            continue
        compiled_patterns.append((description, regex))

    proc = run(
        repo,
        [
            "log",
            *PUBLIC_REVSET,
            "--format=commit:%H",
            "-p",
            "--",
            ".",
            *EXCLUDE_SPECS,
        ],
    )
    if proc.returncode != 0:
        print_fail(f"{name}: unable to scan diff history for local denylist patterns", proc.stderr)
        return failures + 1

    matched_descriptions: set[str] = set()
    current_commit = "<unknown>"
    for line in proc.stdout.splitlines():
        if line.startswith("commit:"):
            current_commit = line.split(":", 1)[1]
            continue
        for description, regex in compiled_patterns:
            if description in matched_descriptions:
                continue
            if regex.search(line):
                print_fail(f"{name}: diff history matched {description}", current_commit)
                matched_descriptions.add(description)
                failures += 1
        if len(matched_descriptions) == len(compiled_patterns):
            break

    return failures


def main() -> int:
    local_patterns = load_local_denylist()
    message_patterns = [*MESSAGE_PATTERNS, *local_patterns]
    diff_patterns = [*DIFF_PATTERNS]

    failures = 0
    for name, repo in PUBLIC_REPOS:
        print(f"INFO: Auditing history for {name}")
        if not (repo / ".git").exists():
            print_fail(f"{name}: repo not found at {repo}")
            failures += 1
            continue
        failures += audit_commit_messages(name, repo, message_patterns)
        failures += audit_path_history(name, repo, local_patterns)
        failures += audit_diff_history(name, repo, diff_patterns)
        failures += audit_local_diff_history(name, repo, local_patterns)

    if failures:
        print(f"\nHistory audit failed with {failures} finding(s).")
        return 1

    print("\nHistory audit passed.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
