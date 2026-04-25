package main

// Desktop-side transparent self-enrolment for the phase-5 per-device
// token model. The desktop runs in the same process as the server, so
// it can read the bootstrap token directly from disk and enrol without
// user interaction. The resulting device token is cached in
// clientdata/device-token.txt so subsequent sessions reuse the same
// token rather than piling up "desktop" entries in devices.json.
//
// Remote clients (browser, secondary laptop) enrol via the same
// POST /auth/enrol endpoint — they just have to paste the bootstrap
// token manually on first run. That UI lands in a future session.

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"bruv/internal/config"
)

const desktopDeviceName = "desktop-local"

// desktopDeviceToken is the cached device token for this process. Set
// once at startup via ensureDesktopDeviceToken. GetHTTPTransportInfo
// reads this; if it's empty the frontend gets an empty token and
// falls back to whatever manual configuration the user has done.
var desktopDeviceToken string

// ensureDesktopDeviceToken resolves the device token for this desktop
// session using the following precedence:
//
//  1. clientdata/device-token.txt (saved from a prior session)
//  2. Self-enrol via the server's DeviceStore using bootstrap-token.txt
//
// Failure is logged but non-fatal — the app still runs, but the
// frontend won't be able to reach /rpc until the user enrols manually.
func (a *App) ensureDesktopDeviceToken() {
	if a.httpServer == nil {
		return
	}

	clientDir, err := config.ClientDataDir()
	if err != nil {
		slog.Warn("desktop enrol: resolve clientdata failed", "err", err)
		return
	}
	tokenPath := filepath.Join(clientDir, "device-token.txt")

	// Case 1: re-use cached device token.
	if data, readErr := os.ReadFile(tokenPath); readErr == nil {
		cached := strings.TrimSpace(string(data))
		if cached != "" && a.httpServer.Devices().LookupDevice(cached) != nil {
			desktopDeviceToken = cached
			slog.Info("desktop enrol: reusing cached device token")
			return
		}
		// Stale token (e.g. devices.json wiped) — fall through to re-enrol.
	}

	// Case 2: self-enrol using the bootstrap token.
	serverDir, err := config.ServerDataDir()
	if err != nil {
		slog.Warn("desktop enrol: resolve serverdata failed", "err", err)
		return
	}
	bootstrapBytes, err := os.ReadFile(filepath.Join(serverDir, "bootstrap-token.txt"))
	if err != nil {
		slog.Warn("desktop enrol: read bootstrap token failed", "err", err)
		return
	}
	bootstrap := strings.TrimSpace(string(bootstrapBytes))

	plaintext, _, err := a.httpServer.Devices().Enrol(bootstrap, desktopDeviceName)
	if err != nil {
		slog.Warn("desktop enrol: enrolment failed", "err", err)
		return
	}

	// Persist for next session so we don't spawn new device entries
	// on every launch.
	if err := os.WriteFile(tokenPath, []byte(plaintext+"\n"), 0o600); err != nil {
		slog.Warn("desktop enrol: cache device token failed", "err", err)
		// Still OK to use this session — just noisy next launch.
	}
	desktopDeviceToken = plaintext
	slog.Info("desktop enrol: issued fresh device token")
}
