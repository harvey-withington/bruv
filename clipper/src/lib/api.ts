// BRUV server client for the extension background worker. Mirrors the mobile
// PWA's transport: POST /auth/enrol with a bootstrap bearer token, GET /repos,
// and JSON-RPC 2.0 with POSITIONAL params against /repos/<id>/rpc. Kept
// dependency-free and platform-blind.

import type { ClipperSettings } from './types'

const SETTINGS_KEY = 'bruv_settings'

export async function loadSettings(): Promise<ClipperSettings | null> {
  const got = await chrome.storage.local.get(SETTINGS_KEY)
  const s = got[SETTINGS_KEY] as ClipperSettings | undefined
  return s && s.serverURL && s.deviceToken ? s : null
}

export async function saveSettings(s: ClipperSettings): Promise<void> {
  await chrome.storage.local.set({ [SETTINGS_KEY]: s })
}

export async function clearSettings(): Promise<void> {
  await chrome.storage.local.remove(SETTINGS_KEY)
}

export type EnrolResult = { serverURL: string; deviceToken: string; deviceID: string }

export async function enrol(serverURL: string, bootstrapToken: string): Promise<EnrolResult> {
  const url = serverURL.trim().replace(/\/+$/, '')
  const token = bootstrapToken.trim()
  if (!url || !token) throw new Error(chrome.i18n.getMessage('options_err_url_and_token'))
  const res = await fetch(`${url}/auth/enrol`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ bootstrap_token: token, device_name: 'BRUV Clipper' }),
  })
  if (!res.ok) {
    let detail = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body?.error) detail = body.error
    } catch { /* keep statusText */ }
    throw new Error(`${res.status} ${detail}`)
  }
  const body = (await res.json()) as { device_token?: string; device_id?: string }
  if (!body.device_token || !body.device_id) throw new Error('malformed enrol response')
  return { serverURL: url, deviceToken: body.device_token, deviceID: body.device_id }
}

// Same-machine pairing — no bootstrap paste. The server honours
// /auth/local-pair only for unproxied loopback requests carrying a
// browser-privileged Origin (see transport/http/localpair.go); this
// extension qualifies via its chrome-extension:// origin. Options shows
// the button only when the entered server URL is loopback.
export async function enrolLocal(serverURL: string): Promise<EnrolResult> {
  const url = serverURL.trim().replace(/\/+$/, '')
  if (!url) throw new Error(chrome.i18n.getMessage('options_err_url_required'))
  const res = await fetch(`${url}/auth/local-pair`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device_name: 'BRUV Clipper' }),
  })
  if (!res.ok) {
    let detail = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body?.error) detail = body.error
    } catch { /* keep statusText */ }
    throw new Error(`${res.status} ${detail}`)
  }
  const body = (await res.json()) as { device_token?: string; device_id?: string }
  if (!body.device_token || !body.device_id) throw new Error('malformed enrol response')
  return { serverURL: url, deviceToken: body.device_token, deviceID: body.device_id }
}

async function apiFetch(s: ClipperSettings, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers)
  headers.set('Authorization', `Bearer ${s.deviceToken}`)
  if (init?.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  return fetch(`${s.serverURL}${path}`, { ...init, headers })
}

export type RepoSummary = { id: string; name: string }

export async function listRepos(s: ClipperSettings): Promise<RepoSummary[]> {
  const res = await apiFetch(s, '/repos')
  if (!res.ok) throw new Error(`list repos: ${res.status}`)
  return ((await res.json()) as RepoSummary[]) ?? []
}

type RPCResponse<T> = {
  jsonrpc: string
  id: number
  result?: T
  error?: { code: number; message: string }
}

let rpcID = 0

export async function repoRPC<T = unknown>(s: ClipperSettings, method: string, params: unknown[]): Promise<T> {
  if (!s.repoID) throw new Error('no repo selected')
  const res = await apiFetch(s, `/repos/${encodeURIComponent(s.repoID)}/rpc`, {
    method: 'POST',
    body: JSON.stringify({ jsonrpc: '2.0', method, params, id: ++rpcID }),
  })
  if (!res.ok) throw new Error(`${method}: HTTP ${res.status}`)
  const payload = (await res.json()) as RPCResponse<T>
  if (payload.error) {
    if (payload.error.code === -32601) throw new Error(`${method}: server too old for this clipper — update BRUV`)
    throw new Error(payload.error.message)
  }
  return payload.result as T
}

// NetworkError-ish check: fetch() rejects with TypeError when the server was
// never reached — that's the "queue it and retry" signal, as opposed to a
// business error which retrying won't fix.
export function isNetworkError(err: unknown): boolean {
  return err instanceof TypeError
}
