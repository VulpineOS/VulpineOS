# Release Hygiene

Run the public boundary audit before cutting a release candidate or after any cross-repo refactor.

Public releases should contain high-level architecture, orchestration, public integration seams, compatibility work, docs, tests, and release hygiene only. Do not ship large product features, larger feature proposals, unreviewed planning detail, implementation detail, or sensitive operational infrastructure in public repos unless the maintainer has explicitly approved that disclosure.

## Command

```bash
./scripts/public-boundary-audit.sh
./scripts/public-history-audit.py
```

The audit scans the tracked files in these public repos:

- `VulpineOS`
- `vulpine-mark`
- `mobilebridge`
- `foxbridge`
- `vulpineos-docs`

It checks for:

- references to `.claude/private-docs`
- references to private repos (`vulpine-private`, `vulpine-api`)
- real local absolute paths
- high-confidence secret tokens
- unsafe `upstream` push configuration

It intentionally skips generated and vendored paths such as `node_modules`, build output, `go.sum`, and public LLM text exports. Placeholder examples like `/home/name` or `C:\Users\<user>` are not treated as leaks.

The history audit scans all reachable commits in the same public repos. It checks commit messages, historical file paths, and text diffs for the same leak patterns before a release tag or release candidate.

The audit does not decide whether a feature is in scope to publish. Review every release diff for out-of-scope implementation details before tagging or pushing public release branches.

## Release checklist

1. Run `./scripts/public-boundary-audit.sh`
2. Run `./scripts/public-history-audit.py`
3. Confirm every public repo is clean with `git status --short`
4. Confirm `VulpineOS` still has `remote.upstream.pushurl=DISABLED`
5. Review release notes and docs for private-code references before tagging

For the broader public release flow, including verification, rebuild,
tagging, and checksum steps, see `docs/release-checklist.md`.
