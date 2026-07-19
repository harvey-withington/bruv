// Cloud adapter — speaks JSON-RPC + Server-Sent Events to the Go
// HTTP transport (see transport/http/). Used for Mode A remote
// backends, Mode B hosted deployments, AND the current desktop Wails
// shell (via loopback HTTP). Replaces the direct Wails bindings in
// every call site that hits the backend.
//
// Bootstrap strategy: the desktop Wails shell exposes a single method
// `GetHTTPTransportInfo` that returns the loopback addr + bearer
// token. That one Wails call is the ONLY thing the cloud adapter
// needs from Wails — after it, every other backend interaction is
// HTTP. When the Wails shell is fully removed (post phase 10) this
// bootstrap switches to env-var or user-provided URL/token.

import type {
  BackendAdapter,
  BackendCapabilities,
  BackendEvent,
  EventCallback,
  UIPreferences,
  WailsWindow,
} from '../types'
import { KNOWN_TOPICS } from './topics'

// --- Bootstrap: get the HTTP transport addr + token ---

type TransportInfo = {
  addr: string
  token: string
  scheme?: string  // "http" (default) or "https" — for TLS-fronted remote servers
  repoID?: string  // active repo ID for the current connection; empty until the user picks a repo
}

// NeedsEnrolmentError is thrown by resolveTransport when the user
// needs to complete the first-run enrolment wizard. main.ts catches
// this and mounts the wizard instead of the main app shell.
export class NeedsEnrolmentError extends Error {
  constructor() {
    super('enrolment required')
    this.name = 'NeedsEnrolmentError'
  }
}

// localStorage keys for browser-mode config. Written by the
// EnrolmentScreen after a successful /auth/enrol call.
const STORAGE_SERVER_URL = 'bruv:server_url'
const STORAGE_DEVICE_TOKEN = 'bruv:device_token'

async function resolveTransport(): Promise<TransportInfo> {
  // 1. Explicit env override — used by browser mode in dev / tests.
  const envURL = import.meta.env.VITE_BRUV_SERVER_URL as string | undefined
  const envToken = import.meta.env.VITE_BRUV_TOKEN as string | undefined
  if (envURL && envToken) {
    const isHttps = envURL.startsWith('https://')
    return {
      addr: envURL.replace(/^https?:\/\//, ''),
      token: envToken,
      scheme: isHttps ? 'https' : 'http',
    }
  }

  // 2. Wails shell — Shell API returns either loopback (normal
  // desktop) or a remote tailnet host (Mode B2, when the desktop
  // was started with BRUV_REMOTE_URL + BRUV_REMOTE_TOKEN env vars).
  // Same code path either way; the scheme hint tells us which.
  const shell = (window as WailsWindow).go?.main?.ShellAPI
  if (shell?.GetHTTPTransportInfo) {
    const info = (await shell.GetHTTPTransportInfo()) as TransportInfo
    if (info.addr && info.token) return info
  }

  // 3. Browser mode — look for a previously-enrolled device token in
  // localStorage. Populated by the EnrolmentScreen after a successful
  // /auth/enrol call.
  try {
    const url = localStorage.getItem(STORAGE_SERVER_URL)
    const token = localStorage.getItem(STORAGE_DEVICE_TOKEN)
    if (url && token) {
      const isHttps = url.startsWith('https://')
      return {
        addr: url.replace(/^https?:\/\//, ''),
        token,
        scheme: isHttps ? 'https' : 'http',
      }
    }
  } catch {
    // localStorage access can throw in Safari private mode etc.
    // Fall through to needs-enrolment.
  }

  // 4. No config anywhere — signal the caller to show the enrolment
  // wizard instead of crashing into an unhappy error screen.
  throw new NeedsEnrolmentError()
}

// saveEnrolment persists the post-/auth/enrol response to
// localStorage so subsequent page loads skip the wizard.
export function saveEnrolment(serverURL: string, deviceToken: string): void {
  localStorage.setItem(STORAGE_SERVER_URL, serverURL)
  localStorage.setItem(STORAGE_DEVICE_TOKEN, deviceToken)
}

// clearEnrolment removes saved credentials — for a future
// "switch server" UI action.
export function clearEnrolment(): void {
  localStorage.removeItem(STORAGE_SERVER_URL)
  localStorage.removeItem(STORAGE_DEVICE_TOKEN)
}

// httpBase builds the scheme-aware base URL for the backend. Honours
// the scheme hint from resolveTransport so Mode B2 against a
// tailnet-TLS-fronted server uses https without hardcoding.
function httpBase(info: TransportInfo): string {
  const scheme = info.scheme === 'https' ? 'https' : 'http'
  return `${scheme}://${info.addr}`
}

// repoPathPrefix returns "/repos/<id>" when a repo is selected on the
// active connection, empty otherwise. Every connection (including
// the desktop's Local loopback) is multi-repo since the local-as-
// remote pivot, so there's no longer a Local-vs-Remote distinction
// in URL shape. Per-machine RPCs route via /server/rpc — see
// SERVER_METHODS below — so callers don't need a repoID for those.
function repoPathPrefix(info: TransportInfo): string {
  if (info.repoID) {
    return `/repos/${info.repoID}`
  }
  return ''
}

// serverGet fetches a server-scoped (non-repo) endpoint. Used by the
// repo-picker to call GET /repos and by the health probe to call
// /healthz — both live above the per-repo URL space.
export async function serverGet(info: TransportInfo, path: string, init?: RequestInit): Promise<Response> {
  const url = `${httpBase(info)}${path}`
  return fetch(url, {
    ...init,
    headers: {
      ...(init?.headers ?? {}),
      Authorization: `Bearer ${info.token}`,
    },
  })
}

// resolveTransportInfo is exposed so screens like the repo picker
// can hit server-scoped endpoints (/repos, /healthz) without going
// through the proxy adapter. Caches the result of resolveTransport
// for the session — same TransportInfo used by initCloudAdapter.
let cachedTransportInfo: TransportInfo | null = null
export async function resolveTransportInfo(): Promise<TransportInfo> {
  if (cachedTransportInfo) return cachedTransportInfo
  cachedTransportInfo = await resolveTransport()
  return cachedTransportInfo
}

// --- Network resilience hook (remote connection loss) ----------------
//
// The shared adapter has no UI and no connection model, so the frontend
// injects a small strategy. When installed, a network-level fetch failure
// on a remote connection parks the RPC until connectivity returns, then
// replays it — so callers' awaits stay pending (optimistic UI intact)
// across the outage instead of throwing. Local (in-process) backends opt
// out via shouldHandleOffline() so a genuine local failure still throws.
export interface NetworkResilience {
  /** True if a network failure right now is a recoverable connection
   *  loss (a remote connection is active). False → throw as-is. */
  shouldHandleOffline(): boolean
  /** Announce that a request just failed at the network layer (show the
   *  overlay, start reconnect probing). Idempotent. */
  onNetworkFailure(): void
  /** Resolves when connectivity is restored (or confirmed never lost). */
  whenReconnected(): Promise<void>
  /** Optional: a request has been pending unusually long. Lets the
   *  strategy probe NOW (without aborting the request) so a dropped
   *  connection surfaces in seconds instead of waiting out the browser's
   *  ~30s+ socket timeout. A slow-but-alive request is unaffected. */
  onSlowRequest?(): void
}

// A remote RPC still pending after this long triggers an early
// reachability probe. Comfortably above normal CRUD latency, so a
// healthy-but-slow request just probes-and-continues.
const SLOW_REQUEST_MS = 5000

let resilience: NetworkResilience | null = null
export function setNetworkResilience(r: NetworkResilience | null): void {
  resilience = r
}

// --- JSON-RPC 2.0 client ---

type RPCError = { code: number; message: string; data?: unknown }

// RPCClient holds two endpoints — per-repo (/repos/<id>/rpc) and
// per-machine (/server/rpc) — and routes each call to one of them
// based on the SERVER_METHODS set. Repo endpoint is only set when a
// repoID is selected; calls without a repoID against per-repo methods
// throw a clear "no repo selected" error rather than 404 on the wire.
class RPCClient {
  private readonly base: string
  private readonly repoEndpoint: string  // "" if no repo selected
  private readonly serverEndpoint: string
  private readonly headers: Record<string, string>
  private nextID = 1
  private versionPromise: Promise<string> | null = null

  constructor(base: string, token: string, prefix: string) {
    this.base = base
    this.repoEndpoint = prefix ? `${base}${prefix}/rpc` : ''
    this.serverEndpoint = `${base}/server/rpc`
    this.headers = {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    }
  }

  // Best-effort server build version (GET /version is unauthenticated and
  // has existed since the first server release). Cached for the client's
  // lifetime; '' when unreachable. Used to make stale-server errors
  // actionable — see the -32601 branch in call().
  private serverVersion(): Promise<string> {
    if (!this.versionPromise) {
      this.versionPromise = fetch(`${this.base}/version`)
        .then(r => (r.ok ? r.json() : null))
        .then(v => (v && typeof v.version === 'string' ? v.version : ''))
        .catch(() => '')
    }
    return this.versionPromise
  }

  async call(method: string, params: unknown[]): Promise<unknown> {
    const endpoint = SERVER_METHODS.has(method) ? this.serverEndpoint : this.repoEndpoint
    if (!endpoint) {
      throw new Error(`rpc ${method}: no repo selected (per-repo method called before pick)`)
    }
    const id = this.nextID++
    const body = JSON.stringify({ jsonrpc: '2.0', method, params, id })
    // Park-and-replay loop: a network-level failure on a remote
    // connection waits for reconnection and resends the same request, so
    // the caller's await stays pending across the outage instead of
    // throwing. HTTP/RPC errors (the server answered) are NOT retried.
    for (;;) {
      let res: Response
      // Watchdog: if this fetch is still pending after SLOW_REQUEST_MS,
      // nudge the strategy to probe — surfaces a dropped connection fast
      // instead of waiting out the native socket timeout. Doesn't abort.
      const watchdog = setTimeout(() => resilience?.onSlowRequest?.(), SLOW_REQUEST_MS)
      try {
        res = await fetch(endpoint, {
          method: 'POST',
          headers: this.headers,
          body,
        })
      } catch (err) {
        clearTimeout(watchdog)
        if (err instanceof TypeError && resilience?.shouldHandleOffline()) {
          resilience.onNetworkFailure()
          await resilience.whenReconnected()
          continue
        }
        throw err
      }
      clearTimeout(watchdog)
      if (!res.ok) {
        throw new Error(`rpc ${method}: HTTP ${res.status} ${res.statusText}`)
      }
      const payload = (await res.json()) as { result?: unknown; error?: RPCError }
      if (payload.error) {
        // "method not found" almost always means the connected server runs
        // an older binary than this frontend (stale deploy / outdated
        // install). Name the version and the fix instead of mystifying.
        if (payload.error.code === -32601) {
          const version = await this.serverVersion()
          const server = version ? `BRUV server ${version}` : 'the connected BRUV server'
          throw Object.assign(
            new Error(`${method} is not supported by ${server} — update the server to use this feature.`),
            { name: 'MethodNotSupportedError', rpcCode: payload.error.code, rpcData: payload.error.data, method, serverVersion: version },
          )
        }
        throw Object.assign(new Error(payload.error.message), {
          rpcCode: payload.error.code,
          rpcData: payload.error.data,
        })
      }
      return payload.result
    }
  }
}

// SERVER_METHODS are per-machine RPCs that route to /server/rpc on
// the active connection — preferences, profile, LLM accounts, etc.
// Reachable before any repo is selected (the picker, the welcome
// flow, the boot-time first-run nudge all live in this state).
// Everything not in this set routes to /repos/<id>/rpc, which
// requires a repo to be picked first.
//
// Keep in sync with core/supervisor/machine.go's MachineService
// surface — adding a method there means adding its name here.
const SERVER_METHODS = new Set<string>([
  'GetPreferences',
  'SetPreferences',
  'GetProfile',
  'SetProfile',
  'GetAuthInfo',
  'MarkLLMNudgeShown',
  'GetLLMConfig',
  'SetLLMConfig',
  'GetLLMAccounts',
  'SaveLLMAccounts',
  'IsLLMConfigured',
  'GetNotifyConfig',
  'SetNotifyConfig',
  'GetNotifications',
  'GetDueDateSettings',
  'SaveDueDateSettings',
  'GetVapidPublicKey',
  'RegisterPushSubscription',
  'UnregisterPushSubscription',
])

// --- Event stream via SSE ---

class EventStream {
  private source: EventSource | null = null
  private readonly url: string
  private readonly listeners = new Set<EventCallback>()
  private reconnectDelay = 500

  constructor(base: string, token: string, prefix: string) {
    // Token on the query string because EventSource can't set headers.
    // Loopback / tailnet-only traffic limits exposure; TLS on the
    // remote-host path keeps it out of on-the-wire view. prefix
    // matches RPCClient — "/repos/<id>" for multi-repo Remote, ""
    // for Local.
    this.url = `${base}${prefix}/events?token=${encodeURIComponent(token)}`
  }

  start(): void {
    if (this.source) return
    const src = new EventSource(this.url)
    this.source = src

    // The Go server uses named SSE events (event: card:updated). We
    // have to subscribe per-topic — addEventListener by name. But
    // the adapter contract is a single callback receiving typed
    // events. Handle this by listening for all known topics and
    // also the default 'message' for any un-named frames.
    for (const topic of KNOWN_TOPICS) {
      src.addEventListener(topic, (ev) => this.dispatch(topic, (ev as MessageEvent).data))
    }

    src.onerror = () => {
      // EventSource reconnects itself automatically; this is just for
      // diagnostic logging. Reset backoff on any open event.
      console.warn('[bruv] SSE connection error — EventSource will retry')
    }
    src.onopen = () => {
      this.reconnectDelay = 500
    }
  }

  stop(): void {
    this.source?.close()
    this.source = null
  }

  on(cb: EventCallback): void {
    this.listeners.add(cb)
  }
  off(cb: EventCallback): void {
    this.listeners.delete(cb)
  }

  private dispatch(topic: string, raw: string): void {
    // Payloads are JSON objects published by the Go side; anything
    // that isn't an object (or fails to parse) carries no fields.
    let fields: Record<string, unknown> = {}
    try {
      const parsed: unknown = JSON.parse(raw)
      if (parsed && typeof parsed === 'object') {
        fields = parsed as Record<string, unknown>
      }
    } catch {
      // Not JSON — dispatch the bare topic event.
    }
    // Spread AFTER stamping the topic would let a payload carrying a
    // `type` JSON field clobber it (model.Card serialises `type`) and
    // silently break every onEvent filter — so stamp last.
    const ev: BackendEvent = { ...fields, type: topic }
    for (const cb of this.listeners) {
      try {
        cb(ev)
      } catch (err) {
        console.error(`[bruv] event listener threw for topic ${topic}:`, err)
      }
    }
  }
}

// --- Adapter construction ---

let rpcClient: RPCClient | null = null
let stream: EventStream | null = null
let wailsShellAvailable = false

function capabilities(): BackendCapabilities {
  return {
    // True when a Wails shell is hosting us — native dialogs,
    // file pickers and shell-open ops route through Wails IPC.
    // Remote-only deployments (no Wails shell) set this false so
    // the UI can hide native-only affordances.
    hasLocalFilesystem: wailsShellAvailable,
    hasAuth: false,            // bearer-token pre-enrolled; user-facing auth deferred to phase 5
    hasRealtime: true,         // SSE connected
  }
}

// Methods that must NOT go through RPC — they're native-shell
// operations that can only execute in the same process as the user's
// GUI. Served by Wails IPC when available, or rejected with a clear
// error message in remote/browser mode.
const SHELL_METHODS = new Set<string>([
  'PickFolder',
  'PickFile',
  'PickSaveFile',
  'OpenConfigFolder',
  'OpenLogsFolder',
  'OpenBugReportURL',
  // Workspace Tier 1 actions — act on files on THIS device.
  'OpenWorkspacePath',
  'RevealWorkspacePath',
  'RunWorkspaceLaunchCommand',
  'ForceQuit',
  // Build info / version — report on the running desktop binary, so they
  // live on the Wails shell rather than the per-repo RPC surface.
  'GetBuildInfo',
  'Version',
  'CheckForUpdates',
])

function buildShellMethod(name: string): (...args: unknown[]) => Promise<unknown> {
  return async (...args: unknown[]) => {
    const binding = (window as WailsWindow).go?.main?.ShellAPI?.[name]
    if (typeof binding !== 'function') {
      throw new Error(
        `${name} is only available inside the Wails desktop shell (not supported in this mode).`,
      )
    }
    return binding(...args)
  }
}

// The RPC-passthrough proxy: every property access that isn't one of
// the adapter's known non-RPC members (getCapabilities, subscribe,
// unsubscribe) becomes a function that forwards args to the RPC
// client. This avoids writing 135 trivial method bodies.
const nonRPCMembers = new Set<string>([
  'getCapabilities',
  'subscribe',
  'unsubscribe',
  'GetUIPreferences',
  'SetUIPreferences',
])

// --- Per-device UI preferences ---------------------------------------
//
// Theme, locale, layout, first-run flags. These must NOT travel over
// RPC — in remote mode that would share one theme across every device.
// Desktop (any mode): served by the local Wails shell, persisted at
// <clientdata>/ui_preferences.json. Browser (no shell): localStorage
// with the same shape and partial-merge semantics.
const UI_PREFS_LS_KEY = 'bruv:ui_preferences'

function defaultUIPreferences(): UIPreferences {
  // Mirrors config.DefaultUIPreferences() in Go.
  return {
    reopen_last_repo: true,
    theme: 'dark',
    locale: 'en',
    confirm_before_delete: true,
    sidebar_width: 260,
    type_badge_display: 'color',
    inbox_recent_cards_limit: 21,
    inbox_activity_limit: 25,
    sidebar_collapse_default: false,
    llm_nudge_shown: false,
  }
}

function uiPrefsShellBinding(name: 'GetUIPreferences' | 'SetUIPreferences') {
  const binding = (window as WailsWindow).go?.main?.ShellAPI?.[name]
  return typeof binding === 'function' ? binding : null
}

async function getUIPreferences(): Promise<UIPreferences> {
  const binding = uiPrefsShellBinding('GetUIPreferences')
  if (binding) return await binding() as UIPreferences
  try {
    const raw = localStorage.getItem(UI_PREFS_LS_KEY)
    if (!raw) return defaultUIPreferences()
    return { ...defaultUIPreferences(), ...JSON.parse(raw) as Partial<UIPreferences> }
  } catch {
    return defaultUIPreferences()
  }
}

async function setUIPreferences(p: Partial<UIPreferences>): Promise<void> {
  const binding = uiPrefsShellBinding('SetUIPreferences')
  if (binding) {
    // The shell takes the partial as a raw JSON string and merges it
    // on the Go side (config.UpdateUIPreferencesPartial).
    await binding(JSON.stringify(p))
    return
  }
  const merged = { ...(await getUIPreferences()), ...p }
  try { localStorage.setItem(UI_PREFS_LS_KEY, JSON.stringify(merged)) } catch { /* private mode / storage full */ }
}

// GetCardProjectContext is a hot path (called per-link during markdown
// rendering). Cache briefly to avoid a burst of identical RPC calls
// when a document contains many inline card references.
const contextCache = new Map<string, { value: string; at: number }>()
const CONTEXT_TTL_MS = 2_000

async function getCardProjectContextCached(cardID: string): Promise<string> {
  const now = Date.now()
  const hit = contextCache.get(cardID)
  if (hit && now - hit.at < CONTEXT_TTL_MS) return hit.value
  const value = (await rpcClient!.call('GetCardProjectContext', [cardID])) as string
  contextCache.set(cardID, { value, at: now })
  return value
}

// signAttachmentURLFull wraps the per-repo SignAttachmentURL RPC.
// The Runtime returns a server-relative path (including its own
// /repos/<id>/ prefix + signed query string); the wrapper prepends
// the active connection's scheme://host so consumers can drop the
// result straight into <img src> / <a href>. Keeps the HMAC secret
// server-side — the client never sees it.
async function signAttachmentURLFull(cardID: string, attachmentID: string): Promise<string> {
  const info = await resolveTransportInfo()
  const path = (await rpcClient!.call('SignAttachmentURL', [cardID, attachmentID])) as string
  return `${httpBase(info)}${path}`
}

// signPresentURLFull — same URL-completing contract as signAttachmentURLFull,
// for the present page. The result is what gets pasted into OBS, so it must
// be absolute.
async function signPresentURLFull(cardID: string): Promise<string> {
  const info = await resolveTransportInfo()
  const path = (await rpcClient!.call('SignPresentURL', [cardID])) as string
  return `${httpBase(info)}${path}`
}

function buildAdapter(): BackendAdapter {
  const base = {
    getCapabilities: capabilities,
    subscribe(cb: EventCallback): void {
      stream?.on(cb)
    },
    unsubscribe(cb: EventCallback): void {
      stream?.off(cb)
    },
    // Hot-path override — caches briefly to avoid RPC storms on
    // markdown documents with many inline bruv:card links.
    GetCardProjectContext: getCardProjectContextCached,
    // URL-completing wrappers — see signAttachmentURLFull for why.
    SignAttachmentURL: signAttachmentURLFull,
    SignPresentURL: signPresentURLFull,
    // Per-device prefs — local shell or localStorage, never RPC.
    GetUIPreferences: getUIPreferences,
    SetUIPreferences: setUIPreferences,
  }

  // Pre-build the shell-method wrappers so every call returns the
  // same function reference (cheaper + better for consumers that
  // might key on identity).
  const shellMethods: Record<string, (...args: unknown[]) => Promise<unknown>> = {}
  for (const name of SHELL_METHODS) {
    shellMethods[name] = buildShellMethod(name)
  }

  const handler: ProxyHandler<typeof base> = {
    get(target: typeof base, prop: string | symbol) {
      if (typeof prop === 'symbol') return Reflect.get(target, prop)
      if (nonRPCMembers.has(prop) || prop === 'GetCardProjectContext' || prop === 'SignAttachmentURL' || prop === 'SignPresentURL') {
        return Reflect.get(target, prop)
      }
      if (shellMethods[prop]) return shellMethods[prop]
      // Promise-protocol probes must return undefined. JS's async
      // runtime duck-types values by checking `.then` before
      // resolving — if our catch-all returns a function for `then`,
      // the returned proxy looks like a thenable, the runtime calls
      // `then(resolve, reject)` as part of `await`, and we end up
      // RPC-ing a method called "then" that the server rightly
      // rejects with "method not found". Same hazard for catch /
      // finally and the Symbol.toPrimitive family handled above.
      if (prop === 'then' || prop === 'catch' || prop === 'finally') {
        return undefined
      }
      // Return a function that forwards positional args to RPC.
      return (...args: unknown[]) => rpcClient!.call(prop, args)
    },
  }

  return new Proxy(base, handler) as unknown as BackendAdapter
}

// --- Exported init ---

export async function initCloudAdapter(): Promise<BackendAdapter> {
  const info = await resolveTransportInfo()
  // Record whether we have a Wails shell hosting us AND we're in
  // loopback mode — shell-method bridge only makes sense when both
  // hold. In Mode B2 the shell is present but the domain server is
  // remote, so native PickFolder etc. still work (they go through
  // the local Wails shell), but hasLocalFilesystem needs to be true
  // here regardless of remote/loopback — the shell IS local.
  wailsShellAvailable = typeof (window as WailsWindow).go?.main?.ShellAPI?.PickFolder === 'function'
  const base = httpBase(info)
  const prefix = repoPathPrefix(info)
  rpcClient = new RPCClient(base, info.token, prefix)
  stream = new EventStream(base, info.token, prefix)
  stream.start()
  return buildAdapter()
}

// Kept for backward compatibility with any test importing the old name.
// Throws a clear error if anyone tries to use it without calling init.
export const cloudAdapter: BackendAdapter = new Proxy({} as BackendAdapter, {
  get(_, prop) {
    throw new Error(
      `cloudAdapter.${String(prop)} accessed before initCloudAdapter() — call initBackend() at app startup.`,
    )
  },
})
