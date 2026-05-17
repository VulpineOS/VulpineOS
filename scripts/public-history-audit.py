#!/usr/bin/env python3

from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
WORKSPACE_ROOT = REPO_ROOT.parent

PUBLIC_REPOS = [
    ("VulpineOS", REPO_ROOT),
    ("vulpine-mark", WORKSPACE_ROOT / "vulpine-mark"),
    ("mobilebridge", WORKSPACE_ROOT / "mobilebridge"),
    ("foxbridge", WORKSPACE_ROOT / "foxbridge"),
    ("vulpineos-docs", WORKSPACE_ROOT / "vulpineos-docs"),
]

EXCLUDE_SPECS = [
    ":(exclude)go.sum",
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

PRIVATE_DOCS_DIR = "." + "claude" + "/private-docs/"
PRIVATE_DOCS_DIR_WINDOWS = "." + "claude" + "\\private-docs\\"
PUBLIC_REVSET = ["--remotes=origin", "--tags"]

MESSAGE_PATTERNS = [
    ("private plan docs", r"\.claude/private-docs(?:/|\\)"),
    ("private repos", r"github\.com/VulpineOS/(vulpine-private|vulpine-api)(?:\b|/)"),
    (
        "high-confidence secret token",
        r"ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,}|lin_api_[A-Za-z0-9]{20,}|xox[pbar]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}|sk-(proj-)?[A-Za-z0-9]{20,}",
    ),
]

DIFF_PATTERNS = [
    ("private plan docs", r"\.claude/private-docs", r"\.claude/private-docs(?:/|\\)"),
    (
        "private repos",
        r"github\.com/VulpineOS/(vulpine-private|vulpine-api)",
        r"github\.com/VulpineOS/(vulpine-private|vulpine-api)(?:\b|/)",
    ),
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


def audit_commit_messages(name: str, repo: Path) -> int:
    proc = run(repo, ["log", *PUBLIC_REVSET, "--format=%H%x00%B%x00==END=="])
    if proc.returncode != 0:
        print_fail(f"{name}: unable to read commit history", proc.stderr)
        return 1

    failures = 0
    text = proc.stdout
    for description, pattern in MESSAGE_PATTERNS:
        regex = re.compile(pattern, re.MULTILINE)
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


def audit_path_history(name: str, repo: Path) -> int:
    proc = run(repo, ["log", *PUBLIC_REVSET, "--name-only", "--format=commit:%H"])
    if proc.returncode != 0:
        print_fail(f"{name}: unable to read path history", proc.stderr)
        return 1

    failures = 0
    lines = proc.stdout.splitlines()
    current_commit = "<unknown>"
    for line in lines:
        if line.startswith("commit:"):
            current_commit = line.split(":", 1)[1]
            continue
        if not line:
            continue
        if PRIVATE_DOCS_DIR in line or PRIVATE_DOCS_DIR_WINDOWS in line:
            print_fail(f"{name}: history contains private plan doc path in {current_commit}", line)
            failures += 1
            break
    return failures


def audit_diff_history(name: str, repo: Path) -> int:
    failures = 0
    for description, pickaxe_pattern, strict_pattern in DIFF_PATTERNS:
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
        strict_regex = re.compile(strict_pattern, re.MULTILINE)
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


def main() -> int:
    failures = 0
    for name, repo in PUBLIC_REPOS:
        print(f"INFO: Auditing history for {name}")
        if not (repo / ".git").exists():
            print_fail(f"{name}: repo not found at {repo}")
            failures += 1
            continue
        failures += audit_commit_messages(name, repo)
        failures += audit_path_history(name, repo)
        failures += audit_diff_history(name, repo)

    if failures:
        print(f"\nHistory audit failed with {failures} finding(s).")
        return 1

    print("\nHistory audit passed.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
