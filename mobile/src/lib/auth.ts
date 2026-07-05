// Mobile auth helpers. Stores the post-enrolment server URL + device
// token (and the chosen repo ID) in localStorage and exposes an
// authenticated `apiFetch` for the rest of the app to use.
//
// The server URL is stored alongside the token because mobile, unlike
// desktop, has no implicit "local" backend — every request goes to
// whichever server the user enrolled with.

import { t } from './i18n.svelte'
import { reportOffline, checkReachable } from './connectivity.svelte'

// A request still pending after this long triggers an early reachability
// probe (without aborting it) so a dropped connection surfaces in seconds
// rather than waiting out the browser's native socket timeout.
const SLOW_REQUEST_MS = 5000

/** Thrown by apiFetch when a request never reaches the server (offline,
 *  DNS, tunnel down). Lets callers tell a connectivity failure apart from
 *  a server-side rejection so they can keep optimistic edits and retry on
 *  reconnect rather than reverting. */
export class NetworkError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'NetworkError'
  }
}

const STORAGE_SERVER_URL = 'bruv:server_url'
const STORAGE_DEVICE_TOKEN = 'bruv:device_token'
const STORAGE_DEVICE_ID = 'bruv:device_id'
const STORAGE_DEVICE_NAME = 'bruv:device_name'
const STORAGE_ACTIVE_REPO = 'bruv:active_repo'

export type EnrolmentResult = {
  serverURL: string
  deviceToken: string
  deviceID: string
  deviceName: string
}

export type StoredEnrolment = {
  serverURL: string
  deviceToken: string
  deviceID: string | null
  deviceName: string | null
}

export function readEnrolment(): StoredEnrolment | null {
  const serverURL = localStorage.getItem(STORAGE_SERVER_URL)
  const deviceToken = localStorage.getItem(STORAGE_DEVICE_TOKEN)
  if (!serverURL || !deviceToken) return null
  return {
    serverURL,
    deviceToken,
    deviceID: localStorage.getItem(STORAGE_DEVICE_ID),
    deviceName: localStorage.getItem(STORAGE_DEVICE_NAME),
  }
}

export function isEnrolled(): boolean {
  return readEnrolment() !== null
}

export function saveEnrolment(result: EnrolmentResult): void {
  localStorage.setItem(STORAGE_SERVER_URL, result.serverURL)
  localStorage.setItem(STORAGE_DEVICE_TOKEN, result.deviceToken)
  localStorage.setItem(STORAGE_DEVICE_ID, result.deviceID)
  localStorage.setItem(STORAGE_DEVICE_NAME, result.deviceName)
}

export function clearEnrolment(): void {
  localStorage.removeItem(STORAGE_SERVER_URL)
  localStorage.removeItem(STORAGE_DEVICE_TOKEN)
  localStorage.removeItem(STORAGE_DEVICE_ID)
  localStorage.removeItem(STORAGE_DEVICE_NAME)
  // Repo selection is meaningless without enrolment — wipe it too.
  localStorage.removeItem(STORAGE_ACTIVE_REPO)
}

// --- Active repo selection -------------------------------------------------

export function readActiveRepoID(): string | null {
  return localStorage.getItem(STORAGE_ACTIVE_REPO)
}

export function saveActiveRepoID(repoID: string): void {
  localStorage.setItem(STORAGE_ACTIVE_REPO, repoID)
}

export function clearActiveRepoID(): void {
  localStorage.removeItem(STORAGE_ACTIVE_REPO)
}

export function hasActiveRepo(): boolean {
  return readActiveRepoID() !== null
}

/**
 * POST a bootstrap token to /auth/enrol on the given server. On success,
 * persists the resulting device token to localStorage and returns the
 * full enrolment result. On failure, throws — the caller surfaces the
 * message to the user.
 */
export async function enrol(args: {
  serverURL: string
  bootstrapToken: string
  deviceName: string
}): Promise<EnrolmentResult> {
  const url = args.serverURL.trim().replace(/\/+$/, '')
  const token = args.bootstrapToken.trim()
  const name = args.deviceName.trim() || defaultDeviceName()

  if (!url) throw new Error('Server URL is required.')
  if (!token) throw new Error('Bootstrap token is required.')

  const res = await fetch(`${url}/auth/enrol`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ bootstrap_token: token, device_name: name }),
  })

  if (!res.ok) {
    let detail = res.statusText
    try {
      const body = await res.json()
      if (body?.error) detail = String(body.error)
    } catch {
      /* fall through to status text */
    }
    throw new Error(`${res.status} ${detail}`)
  }

  const body = (await res.json()) as {
    device_token?: string
    device_id?: string
    device_name?: string
  }
  if (!body.device_token || !body.device_id) {
    throw new Error('Server response missing device token.')
  }

  const result: EnrolmentResult = {
    serverURL: url,
    deviceToken: body.device_token,
    deviceID: body.device_id,
    deviceName: body.device_name ?? name,
  }
  saveEnrolment(result)
  return result
}

/**
 * Make an authenticated request against the enrolled server. Throws if
 * not enrolled. Path is relative to the server root (e.g. '/repos').
 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const enrolment = readEnrolment()
  if (!enrolment) throw new Error('not enrolled')

  const headers = new Headers(init?.headers)
  headers.set('Authorization', `Bearer ${enrolment.deviceToken}`)
  if (init?.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  // Watchdog: if the request is still pending after SLOW_REQUEST_MS, probe
  // reachability early (doesn't abort this request) so a dropped tunnel
  // surfaces the overlay in seconds instead of waiting out the socket
  // timeout. A slow-but-alive request just probes-and-continues.
  const watchdog = setTimeout(() => void checkReachable(), SLOW_REQUEST_MS)
  try {
    return await fetch(`${enrolment.serverURL}${path}`, { ...init, headers })
  } catch (err) {
    // fetch() rejects with a TypeError when the request never reaches
    // the server — offline, DNS failure, or (the common one here) the
    // Tailscale tunnel is down. Raise the global connectivity overlay and
    // surface a friendly, localized message instead of the browser's raw
    // "Failed to fetch" / "Load failed".
    if (err instanceof TypeError) {
      reportOffline()
      throw new NetworkError(t('error.network'))
    }
    throw err
  } finally {
    clearTimeout(watchdog)
  }
}

// JSON-RPC envelope. The Go dispatcher reflects on a target struct
// and calls methods by name with positional args, so `params` is an
// array of arguments in declaration order — NOT an object.
type RPCResponse<T> = {
  jsonrpc: string
  id: number
  result?: T
  error?: { code: number; message: string; data?: unknown }
}

let _rpcID = 0

// Best-effort server build version (GET /version, unauthenticated), cached
// for the session. Used to make stale-server "method not found" errors
// actionable — the fix is updating the server, so say so.
let _serverVersion: Promise<string> | null = null
function serverVersion(): Promise<string> {
  if (!_serverVersion) {
    _serverVersion = apiFetch('/version')
      .then(r => (r.ok ? r.json() : null))
      .then(v => (v && typeof v.version === 'string' ? v.version : ''))
      .catch(() => '')
  }
  return _serverVersion
}

async function rpcCall<T>(endpoint: string, method: string, params: unknown[]): Promise<T> {
  const id = ++_rpcID
  const res = await apiFetch(endpoint, {
    method: 'POST',
    body: JSON.stringify({ jsonrpc: '2.0', method, params, id }),
  })
  if (!res.ok) {
    // Technical detail to the console; localized, status-stamped message
    // to the user (never the raw English statusText).
    console.error(`${method}: HTTP ${res.status} ${res.statusText}`)
    throw new Error(t('error.server', { status: res.status }))
  }
  const payload = (await res.json()) as RPCResponse<T>
  if (payload.error) {
    // -32601 = method not found: the server runs an older binary than
    // this frontend. Name the version and the fix.
    if (payload.error.code === -32601) {
      const version = await serverVersion()
      throw new Error(t('error.method_not_supported', { method, version: version || '?' }))
    }
    throw new Error(`${method}: ${payload.error.message}`)
  }
  // Tolerate both `null` and `undefined` results — some Go methods
  // return zero values that JSON-marshal to null. Caller should `?? []`
  // when expecting an array.
  return payload.result as T
}

/**
 * Make a JSON-RPC call against the active repo's dispatcher.
 * `params` is a positional argument array matching the Go method's
 * signature. Throws if not enrolled, no repo selected, or the server
 * returns an error.
 */
export async function repoRPC<T = unknown>(method: string, params: unknown[] = []): Promise<T> {
  const repoID = readActiveRepoID()
  if (!repoID) throw new Error('no active repo')
  return rpcCall<T>(`/repos/${encodeURIComponent(repoID)}/rpc`, method, params)
}

/**
 * Make a JSON-RPC call against the per-machine dispatcher (preferences,
 * profile, LLM accounts, etc. — anything not scoped to a specific repo).
 */
export async function machineRPC<T = unknown>(method: string, params: unknown[] = []): Promise<T> {
  return rpcCall<T>('/server/rpc', method, params)
}

function defaultDeviceName(): string {
  // Best-effort label so the device list on the server side has
  // something more useful than "Unnamed device". Doesn't need to be
  // unique — the server assigns a UUID.
  const ua = navigator.userAgent || ''
  if (/Android/i.test(ua)) return t('device_name.android')
  if (/iPhone|iPad|iPod/i.test(ua)) return t('device_name.ios')
  return t('device_name.generic')
}
