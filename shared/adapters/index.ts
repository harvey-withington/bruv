import type { BackendAdapter } from '../types'

let _adapter: BackendAdapter

/**
 * Construct the backend adapter. Since phase-4 cleanup there is only
 * one: the cloud adapter, which speaks JSON-RPC + SSE to the Go HTTP
 * transport. Works identically against the Wails-hosted loopback
 * server (desktop mode) and a remote Tailscale-hosted server (Modes
 * A/B in the architecture plan).
 *
 * The `VITE_BACKEND` env var is retained for future transports
 * (e.g. mock for tests). Unknown values fall through to cloud with
 * a console warning rather than throwing — a stale `.env` or build
 * config left over from the Wails-adapter era shouldn't crash the
 * app.
 */
export async function initBackend(): Promise<BackendAdapter> {
  const mode = (import.meta.env.VITE_BACKEND as string | undefined) || 'cloud'
  if (mode !== 'cloud') {
    console.warn(`[bruv] VITE_BACKEND=${mode} is no longer supported — using cloud adapter.`)
  }
  const { initCloudAdapter } = await import('./cloud')
  _adapter = await initCloudAdapter()
  return _adapter
}

export function getBackend(): BackendAdapter {
  if (!_adapter) throw new Error('Backend not initialised — call initBackend() first')
  return _adapter
}

// Test hook: lets vitest install a mock adapter before rendering
// components. Not used in production — real startup always goes
// through initBackend().
export function setBackend(adapter: BackendAdapter): void {
  _adapter = adapter
}
