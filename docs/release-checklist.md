# VulpineOS Release Checklist

This checklist is for public `VulpineOS` release tags and release
candidates.

## Pre-release state

1. Confirm `main` is clean:

   ```bash
   git status --short --branch
   ```

2. Confirm the public remote configuration is still safe:

   ```bash
   git config --get remote.upstream.pushurl
   ```

   Expected value:

   ```text
   DISABLED
   ```

3. Confirm release docs are current:

   - [README.md](../README.md)
   - [docs/release-hygiene.md](release-hygiene.md)
   - any public docs pages or examples changed by the release

## Verification

For Go/runtime changes:

```bash
go build ./internal/... ./cmd/...
go vet ./internal/... ./cmd/...
go test ./internal/... ./cmd/... -race
go build -o vulpineos ./cmd/vulpineos
```

For scoped-session release-candidate coverage, run the soak harness and
keep the JSON artifact with the release notes:

```bash
./scripts/run-soak.sh 3
```

This writes a log plus a small JSON result artifact under
`.artifacts/soak/`.

For Juggler JavaScript changes:

```bash
node --check additions/juggler/protocol/*.js
node --check additions/juggler/content/*.js
```

If the web panel changed:

```bash
cd web
npm run build
```

## Public-boundary checks

Run both public leak audits before tagging:

```bash
./scripts/public-boundary-audit.sh
./scripts/public-history-audit.py
```

These must pass before a release candidate or public tag.

## Browser build status

Record whether the release depends on a fresh Camoufox rebuild.

- If only Go/runtime/docs changed, the rebuilt `./vulpineos` binary is
  enough.
- If Firefox/Juggler patches changed, the release is not complete until a
  new Camoufox build has been produced on the trusted builder path.

For deferred browser rebuild work, link the release notes to the
tracking issue rather than implying the browser binary already contains
the patch set.

For a repeatable off-laptop rebuild path, use
[`docs/ec2-mac-builder.md`](ec2-mac-builder.md).

## Packaging

Before publishing artifacts:

1. rebuild `./vulpineos`
2. compute checksums for shipped archives or binaries
3. verify that release notes and docs do not describe private
   implementation details
4. verify no local/private files are included in the package contents

## Tagging

Create the release tag from `main`:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

## Post-tag checks

Verify:

1. the GitHub tag resolves to the expected commit
2. attached binaries or archives match the published checksums
3. the docs links in the release notes work
4. the public boundary audits still pass from the tagged tree
