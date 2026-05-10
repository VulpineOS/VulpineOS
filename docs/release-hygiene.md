# Release Hygiene

Run the public boundary audit before cutting a release candidate or after any cross-repo refactor.

Public release branches should stay focused on documented public APIs, integration points, compatibility work, docs, tests, and release hygiene. Move unrelated work out of the release branch before tagging.

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

- locally configured sensitive references
- real local absolute paths
- high-confidence secret tokens
- unsafe `upstream` push configuration

It intentionally skips generated and vendored paths such as `node_modules`, build output, `go.sum`, and public LLM text exports. Placeholder examples like `/home/name` or `C:\Users\<user>` are not treated as leaks.

The boundary and history audits also load optional local denylist patterns from `.public-boundary-denylist.local`, or from the file pointed at by `VULPINE_PUBLIC_AUDIT_DENYLIST`. This file is intentionally untracked. Each non-comment line may be either a regex pattern or `description<TAB>regex pattern`.

The history audit scans all reachable commits in the same public repos. It checks commit messages, historical file paths, and text diffs for the same leak patterns before a release tag or release candidate.

The audit does not decide whether a change belongs in this repository. Review every release diff for scope before tagging or pushing public release branches.

## Release checklist

1. Run `./scripts/public-boundary-audit.sh`
2. Run `./scripts/public-history-audit.py`
3. Confirm every public repo is clean with `git status --short`
4. Confirm `VulpineOS` still has `remote.upstream.pushurl=DISABLED`
5. Review release notes and docs for private-code references before tagging

For the broader public release flow, including verification, rebuild,
tagging, and checksum steps, see `docs/release-checklist.md`.
