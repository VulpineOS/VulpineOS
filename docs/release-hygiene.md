# Release Hygiene

Run the public boundary audit before cutting a release candidate or after any cross-repo refactor.

## Command

```bash
./scripts/public-boundary-audit.sh
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

## Release checklist

1. Run `./scripts/public-boundary-audit.sh`
2. Confirm every public repo is clean with `git status --short`
3. Confirm `VulpineOS` still has `remote.upstream.pushurl=DISABLED`
4. Review release notes and docs for private-code references before tagging
