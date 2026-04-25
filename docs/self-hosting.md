# Self-hosting BRUV

BRUV ships as a single Windows binary that can run two ways:

- **Desktop app** — what you get when you double-click `bruv.exe`. The app you use day-to-day.
- **Server** — the same `bruv.exe` running headless in the background, serving every device on your network. Other BRUV installs (laptop, phone via the browser, partner's PC) can point at it and share one repo.

This guide walks through the second mode.

> **Posture:** "Plex for productivity," not Kubernetes. One always-on machine in your home, one repo, the rest of the household pointing at it. No Docker, no nginx config, no certificates to wrangle. Tailscale handles the network plumbing.

---

## What you'll need

- A Windows machine that's powered on most of the time (an old laptop on a shelf, a NUC, a desktop PC). This is the "server" — it doesn't need to be beefy. BRUV is a JSON file mover with a small SQLite index; idle CPU and a few hundred MB of RAM is plenty.
- [Tailscale](https://tailscale.com/download/windows) installed on the server **and** every device that will connect to it. Tailscale is free for personal use and gives every device a stable `100.x.y.z` address that routes between your devices regardless of network. BRUV doesn't ship with Tailscale and doesn't talk to its API — it just listens on `0.0.0.0` and lets Tailscale + your ACLs do the gating.
- The latest BRUV installer from the [Releases page](https://github.com/harvey-withington/bruv/releases).

---

## On the server machine

1. **Install Tailscale**, sign in, confirm the machine appears on your tailnet.
2. **Run the BRUV installer.** On the components page, tick **Server (run in background, auto-start on boot)**. You can leave **Desktop app** ticked too if you also want to use BRUV on this machine; un-tick it for headless boxes.
3. **Click Install.** The installer will:
   - Drop `bruv.exe` into `Program Files`.
   - Create an empty BRUV repo at `%APPDATA%\bruv\server-repo` (you can put it elsewhere later — see [moving the repo](#moving-the-repo)).
   - Register a Windows Service named **BRUV Server**, set to auto-start on boot.
   - Start the service immediately.
4. **Note the connection token.** The finish page tells you where to find the *bootstrap token* the other devices will need to enrol:

   ```
   %APPDATA%\bruv\bootstrap-token.txt
   ```

   This is a one-time-use seed: each device exchanges it for its own long-lived token via `/auth/enrol`, and the bootstrap token isn't needed again after enrolment.

5. **Find the server's URL.** Open Tailscale's tray icon → click the server machine → copy its `100.x.y.z` IP. Your server URL is:

   ```
   http://100.x.y.z:9870
   ```

That's it. The service auto-starts on boot; you don't need to keep an interactive session open.

---

## On every other device

1. **Install Tailscale**, sign in, confirm the device appears on your tailnet alongside the server.
2. **Run the BRUV installer.** Leave **Desktop app** ticked, **un-tick** Server (other devices are clients).
3. **Launch BRUV.** Open the app once so the desktop UI is running. By default it'll connect to its own loopback backend (the implicit "Local" connection).
4. **Add the server connection.**
   - Click the **connection indicator** in the bottom-left of the sidebar (the icon next to the gear and theme toggle). It says "Local" today.
   - Click **Add a server…** in the popover.
   - Fill in:
     - **Name** — any friendly label ("Family Server", "Home NAS", whatever).
     - **Server URL** — `http://100.x.y.z:9870` from the server's Tailscale IP.
     - **Connection Token** — paste the contents of `bootstrap-token.txt` from the server.
   - Click **Add and switch**.
5. The app reloads connected to the server. The connection indicator now shows your server's name. Every card you create or edit lives on the server's repo and is visible from every other device that's enrolled.

---

## Day-two operations

### Switching between connections

The connection indicator in the sidebar always shows the active connection. Click it to switch between Local and any remote you've added — useful for "test it locally first" or "I'm at my partner's place, point at *their* server."

### Removing a connection

Same indicator → **Edit connections…** → trash icon next to the entry. The device forgets the URL and token; the server doesn't know or care.

### Service control

From a terminal on the server machine:

```
bruv.exe service status     # is the server running?
bruv.exe service stop       # stop it
bruv.exe service start      # start it
bruv.exe service restart    # bounce it (e.g. after editing settings)
```

Or use Windows' Services app (`services.msc`) and look for **BRUV Server**.

### Moving the repo

The server expects exactly one repo, picked at install time. To move it:

```
bruv.exe service uninstall
move %APPDATA%\bruv\server-repo D:\BRUV
bruv.exe service install --repo D:\BRUV
```

### Sharing the bootstrap token safely

The bootstrap token grants enrolment rights — anyone who has it can register a device with your server. Treat it like a Wi-Fi password: send it to family over a private channel (Signal, AirDrop, scribble it on paper). Once a device is enrolled it gets its own per-device token; the bootstrap token can be rotated by deleting `bootstrap-token.txt` and restarting the service.

### Backups

The server's data lives at the repo path you picked (default: `%APPDATA%\bruv\server-repo`). It's plain JSON files. Back it up with whatever you already use for backups — Time Machine, Backblaze, robocopy to a USB drive. The `.bruv/` subfolder is derived state (SQLite index, lock file) and doesn't need to be backed up; everything else does.

---

## Troubleshooting

### "I can't reach the server from another device"

- Both devices are on the same tailnet? Check Tailscale's tray icon on both.
- Server machine's Windows Firewall is blocking port 9870? Add an inbound rule for TCP 9870 — Tailscale's interface respects Windows Firewall like any other network.
- Server is actually running? `bruv.exe service status` on the server machine.

### "I get 'connection token rejected' when trying to enrol"

- Tokens are one-time. If you used it once already (even on a different device), generate a fresh one: stop the service, delete `bootstrap-token.txt`, start the service. The service writes a new token on first request.

### "I want to start over"

```
bruv.exe service uninstall
rmdir /s /q %APPDATA%\bruv\server-repo
```

Then re-run the installer with the Server checkbox.

---

## Explicit non-goals

So you don't waste time looking for these:

- **No Docker / Kubernetes / docker-compose.** The whole point of this story is "double-click an installer, done." If you want containerized BRUV, file an issue — it's not hard but no one's asked yet.
- **No HTTPS termination.** BRUV listens on plain HTTP. Tailscale-serve wraps in TLS for free if you really want a public URL; the BRUV server itself stays HTTP and only listens on tailnet/loopback addresses.
- **No multi-user permissions.** Every enrolled device can do anything. This is fine for "all my family's devices" but not for "a SaaS with paying customers." The architecture has the hooks for per-user auth (we just don't ship UI for it).
- **No Mac or Linux server build.** Not yet. The code is cross-platform-clean (kardianos/service abstracts the SCM/launchd/systemd plumbing); the limiter is signed-build infrastructure on Mac and packaging conventions on Linux.
