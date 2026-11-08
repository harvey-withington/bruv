---
name: sync-uncli
description: Bring a new UNCLI build into BRUV — repack UNCLI's chat tarballs (@uncli/client, @uncli/chat), reinstall them in the root, frontend/ and mobile/, check the Go side still builds against the UNCLI checkout, verify, and suggest the commit. Use when UNCLI has changed and BRUV should pick it up, or when asked to "sync uncli", "update the uncli tarballs", "new uncli build", "pull in uncli".
---

# Sync a new UNCLI build into BRUV

BRUV's chat engine is UNCLI, embedded from a local UNCLI checkout until UNCLI is published (UNCLI decision 0026, `docs/EMBEDDING.md` in the UNCLI repo). Two seams carry it:

- **Go:** `go.mod` has `replace github.com/harvey-withington/uncli/app => ../../UNCLI/UNCLI-1.0/app`, so BRUV builds against whatever the checkout holds. The one place BRUV imports it is `internal/uncliengine` (package `uncli`).
- **npm:** the chat element ships as two tarballs that UNCLI packs into `app/frontend/dist-embed/packs/` (ignored by UNCLI's git). BRUV installs them with `file:` paths in three places: the root `package.json` (shared/ imports the element's types), `frontend/` and `mobile/`.

A tarball rebuilt at the same version keeps its name, so npm doesn't notice it changed. The lock files pin each tarball's `integrity` hash, so they change on every real sync. `scripts/sync-uncli.ps1` does the repack and the three reinstalls; this skill is the procedure around it.

## Before

1. **UNCLI is where `go.mod` expects it:** `../../UNCLI/UNCLI-1.0` from the repo root, or set `BRUV_UNCLI_DIR`.
2. **Note what you're syncing:** `git -C <uncli> log -1 --format=%h` gives the sha for the commit message, and `git -C <uncli> status --short` should be empty. The tarballs are built from UNCLI's working tree, so a dirty tree means the sync can't be reproduced from a commit. If it's dirty, say so in the recap rather than refusing.
3. **BRUV's tree is clean,** so the sync's diff is only the sync.

## Run it

From the repo root, in PowerShell:

```powershell
./scripts/sync-uncli.ps1
```

It packs (`npm run pack:packages` in UNCLI's `app/frontend`, which builds the element first), removes `node_modules/@uncli` and reinstalls the two tarballs in `frontend/`, `mobile/` and then the root (with `--ignore-scripts`; its postinstall would reinstall both surfaces again). The root goes last because it links `bruv-frontend` (`file:frontend`), so its install also resolves frontend's `package.json`; after a version bump that must already name the new tarballs, or npm looks for the deleted old ones (ENOENT). Then it re-checks the desktop's vendored dotlottie wasm, which a nested reinstall once lost.

`-SkipPack` reinstalls the tarballs already in `packs/` without rebuilding, e.g. when UNCLI's session already packed them.

## When UNCLI's package version changed

The tarball names carry the version (`uncli-chat-<version>.tgz`). After a bump:

- **Old tarballs stay in `packs/`** and the script stops with "Expected the client and chat tarballs … found: …". Delete the old-version files (they're build output) and run again with `-SkipPack`.
- **`npm install` rewrites the `file:` paths** in all three `package.json` files. Check that all three name the new version, and that nothing else in them changed.

## The Go side

The replace needs no step, but UNCLI's Go changes land in BRUV's next build, so check them now:

```powershell
go build ./...
```

- **"missing go.sum entry":** UNCLI added a Go dependency. Run `go mod tidy`, then check the `go.mod` diff only adds requirements (no version downgrades, no removed `replace`).
- **Compile errors in `internal/uncliengine`:** UNCLI's API changed. Fix it at that seam; don't reach into other UNCLI packages from elsewhere in BRUV.

## Verify

```powershell
npm run check
npm test
```

Both cover the embed (`shared/uncliChat.test.ts`, the uncliengine Go tests). Then **see it rendered**: open a card chat with the chat engine set to UNCLI (Settings → AI) on the desktop, and on the phone if UNCLI's change touches the phone layout. Tests don't cover the element's own UI.

## Commit

Never commit or push unless asked (see `bruv-conventions`). The expected diff:

- the three `package-lock.json` files (new `integrity` hashes); always, for a real change;
- the three `package.json` files, only after a version bump;
- `go.mod` / `go.sum`, only if `go mod tidy` was needed;
- BRUV code, only if UNCLI's API changed.

Suggest a message that names the UNCLI commit and says what changed **for BRUV**, in the history's form:

```
uncli <sha> tarballs: <what changed for bruv>
```

If the lock files are the only change and UNCLI's commit didn't touch the embed, there's nothing worth committing. Say so instead.

## After

- **Home server:** the deploy builds on this machine, so it picks up the new packs and UNCLI's Go code. Deploy only when asked (`deploy-server` skill).
- **Other dev machines:** the tarballs aren't in either repo. After pulling both repos, each machine runs `./scripts/sync-uncli.ps1` itself, with UNCLI at the same commit. It hasn't been checked whether packing the same commit on two machines gives byte-identical tarballs. If the lock files change on a machine that only synced, treat that as build noise, not a change to commit.
- **CI:** the workflows don't check out UNCLI, so on a runner neither the `replace` nor the `file:` tarballs resolve. CI can't build BRUV until UNCLI is published or the workflows fetch and pack it. A red CI after a sync is expected until then; it says nothing about the sync.

## Recap to give

The UNCLI sha (and whether its tree was clean), the old and new package versions, which files changed, whether `go mod tidy` or code changes were needed, check and test results, and whether it was seen rendered.
