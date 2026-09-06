---
name: deploy-server
description: Deploy the BRUV server binary to RIPPED (the home server) — build, push over SSH/Tailscale, swap the BRUV-Server service, verify /version. Use after any Go change that needs to be live for the phone or remote connections, or when asked to "deploy", "redeploy", "push to ripped".
---

# Deploy to RIPPED

RIPPED is the home server (Beelink box, Tailscale host `ripped.tail2ebd58.ts.net`) running the headless `BRUV-Server` Windows service on port 9870. The phone PWA and every Remote connection talk to it, so **any Go change is invisible on those surfaces until RIPPED is redeployed**. Frontend-only or mobile-only changes still ship inside the same unified binary, so they need a deploy too.

The whole procedure is `scripts/deploy-server.ps1`. This skill is how to drive it safely.

## Before deploying

1. **Confirm with Harvey first.** A deploy restarts the live service. Don't run it as a side effect of a code change; run it when asked, or ask once and wait.
2. **Tests green:** `go test ./core/... ./internal/...` (plus `svelte-check` on both surfaces if TS changed — CI runs `--fail-on-warnings` on mobile).
3. **Commit state.** The build stamps `dev-<short sha>` from `git rev-parse HEAD`. Deploying uncommitted work is allowed but the version string will point at the wrong commit — say so in the recap. Harvey commits manually; never commit on his behalf.

## Run it

PowerShell, from the repo root. Always pass `-Force`: the script otherwise prompts with `Read-Host`, which fails in a non-interactive shell.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\deploy-server.ps1 -RemoteHost ripped.tail2ebd58.ts.net -SshUser beelink -Force
```

Redeploy the last build without rebuilding (e.g. after a failed swap):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\deploy-server.ps1 -RemoteHost ripped.tail2ebd58.ts.net -SshUser beelink -SkipBuild -Force
```

Equivalent VS Code tasks: **deploy: server to home** and **deploy: server to home (skip build)**. Env-var defaults: `BRUV_DEPLOY_HOST`, `BRUV_DEPLOY_USER`.

Allow up to ~10 minutes: `wails build` dominates. Use a long Bash/PowerShell timeout or run in the background.

## What the script does

1. `wails build -platform windows/amd64` → `build/bin/bruv-1.0.exe`, version `dev-<sha>`.
2. `scp` the exe to `~/bruv-deploy.exe` on RIPPED.
3. Over `ssh`, as an encoded PowerShell command: read the service's own binary path from `Win32_Service`, stop `BRUV-Server`, back up to `.bak`, copy the new exe (retries on file locks), start the service.
4. On any failure: roll back to `.bak` and restart the service so the box is never left headless.
5. Poll `http://ripped.tail2ebd58.ts.net:9870/version` and print `version` + `build_date`.

## Verify

The final line must be `Server is up: version=dev-<sha> ...` with the sha you just built. If it says the service started but the health URL was unreachable, hit the URL yourself before declaring success:

```powershell
Invoke-RestMethod http://ripped.tail2ebd58.ts.net:9870/version
```

## One-time setup per laptop (BRUV is developed on three)

The script's `scp`/`ssh` run non-interactively, so both the host key and key-based auth must already be in place. Probe first:

```powershell
ssh -o BatchMode=yes beelink@ripped.tail2ebd58.ts.net "echo ok"
```

- `Host key verification failed` → run `ssh beelink@ripped.tail2ebd58.ts.net` once in a real terminal and accept the fingerprint.
- `Permission denied (publickey,password,...)` → this laptop has no key authorised on RIPPED. `beelink` is an Administrator, so Windows OpenSSH reads `C:\ProgramData\ssh\administrators_authorized_keys`, not `~\.ssh\authorized_keys`, and that file must have a restricted ACL or sshd ignores it.

  **Harvey hates terminal commands** (see memory). Claude does everything it can itself and hands over only the steps that need his password. Hand-over rules: one command per block, no comment lines inside blocks, say which window (laptop `PS` prompt vs `beelink@RIPPED` prompt), say what success looks like. Two blocks pasted together broke ssh-keygen on 2026-09-06.

  Claude does (no password needed): `ssh-keygen -t ed25519 -N '""' -f $env:USERPROFILE\.ssh\id_ed25519` if no key exists, then reads `id_ed25519.pub`. The key MUST have no passphrase — the deploy runs unattended, and a passphrased key fails with "Server accepts key" followed by "Permission denied" (2026-09-06: Harvey typed a passphrase at the prompt; the old pair is kept as `id_ed25519.passphrase.bak`). Never have Harvey run ssh-keygen; Claude generates the key.

  Harvey does, in a NEW PowerShell window on the laptop (password prompted; prompt shows `beelink@RIPPED` afterwards):

  ```powershell
  ssh beelink@ripped.tail2ebd58.ts.net
  ```

  Then, in that RIPPED session, one at a time, with the literal public key Claude printed pasted in place of `<PUBKEY>`. **RIPPED's SSH session is cmd.exe, not PowerShell** (prompt `beelink@RIPPED C:\Users\beelink>`), so only cmd syntax works there. No space before `>>` or the key line gets a trailing space.

  ```
  echo <PUBKEY>>>C:\ProgramData\ssh\administrators_authorized_keys
  ```

  ```
  icacls C:\ProgramData\ssh\administrators_authorized_keys /inheritance:r /grant Administrators:F /grant SYSTEM:F
  ```

  ```
  exit
  ```

  Claude then re-runs the probe; it must print `ok` with no prompt before the deploy script will work from Claude.

## Troubleshooting

- **scp hangs forever on "Copying binary to …" with the host key already accepted:** password prompt the shell can't show. Key auth is missing — see the setup section above. Kill the stuck `scp`/`ssh` processes, then rerun with `-SkipBuild`.

- **scp hangs forever on "Copying binary to …" (first deploy from a laptop):** the host key isn't in this machine's `known_hosts`, so ssh is sitting on a yes/no prompt that a non-interactive shell can't show. Probe with `ssh -o BatchMode=yes beelink@ripped.tail2ebd58.ts.net "echo ok"`; "Host key verification failed" confirms it. Fix once per laptop, in a real terminal: `ssh beelink@ripped.tail2ebd58.ts.net` and accept the fingerprint (a password prompt after that means this laptop's key isn't in the server's `authorized_keys` either). Kill the stuck `scp`/`ssh` processes, then rerun with `-SkipBuild`. This bit the 2026-09-06 deploy.
- **scp/ssh fails:** Tailscale down, or OpenSSH not reachable. Check `tailscale status` and that `ssh beelink@ripped.tail2ebd58.ts.net` connects.
- **"copy locked" repeats then FAILED:** something still holds the exe (AV scan, a desktop BRUV instance on the box). The script rolls back; retry with `-SkipBuild`.
- **Service stopped after a failed deploy:** the script already tries to restart it. If it printed the `sc start` warning, run `ssh beelink@ripped.tail2ebd58.ts.net "sc start BRUV-Server"`.
- **Version unchanged after deploy:** the swap rolled back silently or the health check hit a cached response. Re-read the script output for `rolled back`.

## Recap to give Harvey

State the deployed version string, whether the health check confirmed it, and whether the build included uncommitted changes.
