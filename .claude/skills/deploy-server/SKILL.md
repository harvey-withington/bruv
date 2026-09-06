---
name: deploy-server
description: Deploy the BRUV server binary to a self-hosted Windows home server — build, push over SSH, swap the BRUV-Server service, verify /version. Use after any Go change that must be live for the phone PWA or remote connections, or when asked to "deploy", "redeploy", "push to the server".
---

# Deploy BRUV-Server to a home server

BRUV's phone PWA and every Remote connection talk to a headless `BRUV-Server` Windows service on a box you run at home (typically reached over Tailscale). **Any Go change is invisible on those surfaces until that box is redeployed.** Frontend and mobile changes ship inside the same unified binary, so they need a deploy too.

The whole procedure is `scripts/deploy-server.ps1`. This skill is how to drive it safely and unattended.

## Target

The script takes `-RemoteHost` and `-SshUser`, defaulting to the `BRUV_DEPLOY_HOST` and `BRUV_DEPLOY_USER` environment variables. Nothing about a specific server is hardcoded in the repo. If you are an AI assistant running this, take the host and user from the developer's own notes or ask; do not guess. The VS Code tasks **deploy: server to home** and **deploy: server to home (skip build)** prompt for the host.

## Before deploying

1. **Confirm with the developer first.** A deploy restarts the live service. Don't run it as a side effect of a code change; run it when asked, or ask once and wait.
2. **Tests green:** `go test ./core/... ./internal/...`, plus `svelte-check` on both surfaces if TypeScript changed (CI runs `--fail-on-warnings` on mobile).
3. **Commit state.** The build stamps `dev-<short sha>` from `git rev-parse HEAD`. Deploying uncommitted work is allowed, but the version string will point at the wrong commit — say so in the recap.

## Run it

PowerShell, from the repo root. Always pass `-Force`: without it the script prompts with `Read-Host`, which fails in a non-interactive shell.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\deploy-server.ps1 -RemoteHost <host> -SshUser <user> -Force
```

Redeploy the last build without rebuilding (after a failed swap, or when the binary is already built):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\deploy-server.ps1 -RemoteHost <host> -SshUser <user> -SkipBuild -Force
```

Allow up to ten minutes: `wails build` dominates (about 90 seconds on a fast laptop, longer cold). Use a long timeout or run in the background.

## What the script does

1. `wails build -platform windows/amd64 -nsis` → `build/bin/bruv-1.0.exe` plus `build/bin/bruv-amd64-installer.exe`, version `dev-<sha>`. NSIS must be installed locally (the script finds `makensis` in the standard install dirs if it isn't on PATH); without it the build proceeds and warns that the installer was skipped.
2. `scp` the exe to `~/bruv-deploy.exe` on the server, and the installer to `-RemoteInstallerPath` (env `BRUV_DEPLOY_INSTALLER_PATH`; default `bruv-amd64-installer.exe` in the SSH user's home) — a known place so any machine on the tailnet can fetch a matching installer with `scp <user>@<host>:<path> .`. An installer copy failure warns; it never aborts the service swap. `-SkipInstaller` skips both build and push.
3. Over `ssh`, as an encoded PowerShell command: read the service's own binary path from `Win32_Service`, stop `BRUV-Server`, back up to `.bak`, copy the new exe (retries on file locks), start the service.
4. On any failure: roll back to `.bak` and restart the service so the box is never left headless.
5. Poll `http://<host>:9870/version` and print `version` + `build_date`, then confirm the installer refresh.

## Verify

The final line must be `Server is up: version=dev-<sha> ...` with the sha you just built. If it says the service started but the health URL was unreachable, hit the URL yourself before declaring success:

```powershell
Invoke-RestMethod http://<host>:9870/version
```

Pass `-HealthUrl https://<host>/version` if the server is TLS-fronted.

## One-time setup per development machine

`scp`/`ssh` run non-interactively, so the host key and key-based auth must already be in place. Probe first:

```powershell
ssh -o BatchMode=yes <user>@<host> "echo ok"
```

It must print `ok` with no prompt. Otherwise:

- **`Host key verification failed`** → the server isn't in this machine's `known_hosts`. Run `ssh <user>@<host>` once in a real terminal and accept the fingerprint.
- **`Permission denied (publickey,password,...)`** → this machine has no key authorised on the server. Generate one **with no passphrase** (the deploy is unattended; a passphrased key fails with "Server accepts key" followed by "Permission denied"):

  ```powershell
  ssh-keygen -t ed25519 -N '""' -f $env:USERPROFILE\.ssh\id_ed25519
  ```

  Then install the contents of `id_ed25519.pub` on the server. The SSH user must be a local **Administrator** (the swap touches Program Files and the service), and for administrators Windows OpenSSH reads `C:\ProgramData\ssh\administrators_authorized_keys`, not `~\.ssh\authorized_keys`. That file must carry a restricted ACL or sshd silently ignores it. Note that an SSH session into a Windows box lands in **cmd.exe, not PowerShell**, so use cmd syntax there (no space before `>>`, or the key line gets a trailing space):

  ```
  echo <contents of id_ed25519.pub>>>C:\ProgramData\ssh\administrators_authorized_keys
  ```

  ```
  icacls C:\ProgramData\ssh\administrators_authorized_keys /inheritance:r /grant Administrators:F /grant SYSTEM:F
  ```

  Re-run the probe.

If you are an AI assistant walking a developer through this: do everything that needs no password yourself (key generation, probes), hand over only the password-gated steps, one command per block, and say which window each belongs in (local PowerShell vs the server's cmd prompt).

## Troubleshooting

- **scp hangs forever on "Copying binary to …":** it is waiting on a prompt the shell can't show — host key (first connect) or password (no key auth). Both are covered by the setup section. Kill the stuck `scp`/`ssh`, fix, rerun with `-SkipBuild`.
- **scp/ssh fails immediately:** Tailscale or LAN down, or OpenSSH Server not running on the box. Check `tailscale status` and that a plain `ssh <user>@<host>` connects.
- **"copy locked" repeats, then FAILED:** something still holds the exe (AV scan, a desktop BRUV instance on the box). The script rolls back; retry with `-SkipBuild`.
- **Service stopped after a failed deploy:** the script already tries to restart it. If it printed the `sc start` warning, run `ssh <user>@<host> "sc start BRUV-Server"`.
- **Version unchanged after deploy:** the swap rolled back silently. Re-read the script output for `rolled back`.

## Recap to give the developer

State the deployed version string, whether the health check confirmed it, and whether the build included uncommitted changes.
