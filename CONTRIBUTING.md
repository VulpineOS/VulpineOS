# Contributing to VulpineOS

Thanks for your interest in contributing! Here's how to get started.

## Ways to Contribute

- **Bug reports** — Open an issue with steps to reproduce, expected behavior, and actual behavior.
- **Feature requests** — Open an issue describing the use case and why it's useful.
- **Code contributions** — Fork the repo, make your changes, and open a pull request.
- **Documentation** — Fixes and improvements to docs are always welcome.

## Development Setup
See README.md

## Pull Request Rules

1. Each pull request must be associated with a Github issue
2. Follow the pull request template
3. Keep commits focused — one logical change per commit.
4. Open a PR with a clear description of what you changed and why.
5. All pull requests must pass the relevant checks for the files changed before merging.

## Repository Scope

This repository accepts changes to documented public APIs, integration points, compatibility fixes, documentation, tests, and release hygiene. Larger feature proposals should start as an issue so maintainers can confirm scope and repository placement before code is submitted.

## Testing Requirements

For most VulpineOS runtime changes, run:

```bash
go test ./cmd/... ./internal/...
```

If you change the web panel, also run:

```bash
npm --prefix web test -- --run
npm --prefix web run build
```

Browser-layer and service-level validation gates are maintained outside this
public repository. Public contributors should open an issue before proposing
Firefox/Camoufox patch, browser behavior, or Python package integration changes
so maintainers can confirm scope and run the appropriate private gates.

## Reporting Issues

Please search existing issues before opening a new one. Include:
- VulpineOS commit or release version
- Browser binary path/version, if the bug involves a live browser
- OS, Go version, and Node.js version
- Exact run command and a minimal reproducible example

## Questions

For usage questions, check the [documentation](https://docs.vulpineos.com) first. For anything else, open an issue.
