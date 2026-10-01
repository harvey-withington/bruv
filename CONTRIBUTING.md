# Contributing to BRUV

Thanks for taking an interest. BRUV is a small, single-maintainer project built in the open, and contributions — code, bug reports, feedback — are welcome.

## Code of conduct

Be kind. Assume good faith. Disagree with ideas, not people. That's the whole thing.

## Tech stack

- **Desktop shell:** [Wails v2](https://wails.io/) (Go backend + web frontend, single native binary)
- **Frontend:** [Svelte 5](https://svelte.dev/) with runes + TypeScript + Vite
- **Backend:** Go — repository I/O, SQLite indexing, LLM provider adapters, agent runtime
- **LLM:** provider-agnostic (Anthropic, OpenAI, Ollama — more welcome)
- **Storage:** plain JSON files in a repo folder the user picks (portable, shareable), plus personal state in the OS config directory (`%APPDATA%\bruv\` on Windows) — see [Repo format contract](#repo-format-contract)

## Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- **Wails CLI — version must match the pin in [go.mod](go.mod).** Currently `v2.10.1`. Install with:
  ```powershell
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
  ```
  Verify with `wails version`. **A version mismatch will silently rewrite `go.mod` / `go.sum` on every `wails dev` or `wails build`** — the CLI auto-bumps the project's Wails dependency to its own version, which then fails to compile if the corresponding `go-webview2` is incompatible (this manifests as `cannot use f.processMessage` and `GetSource undefined` errors). If you ever see those errors, check `wails version` first.

### Windows-specific

- **PowerShell execution policy** must allow local scripts, otherwise `npm`/`vite`/`tsc` fail with `running scripts is disabled on this system`. One-time fix:
  ```powershell
  Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
  ```

## First-time setup on a new machine

The repo has three `package.json` files: at the **root** (so `shared/*.ts` can resolve imports like `marked`), in **`frontend/`** (the desktop UI), and in **`mobile/`** (the PWA). The root's `postinstall` cascades into the other two, and Wails' [frontend:install](wails.json) hook drives the root install — so on a clean checkout, the first `wails dev` populates all three `node_modules/` automatically.

If you ever need to install manually (e.g. running `go test` without `wails dev`), one command at the root suffices:

```powershell
npm install   # at the repo root — cascades to frontend/ and mobile/ via postinstall
```

Diagnostic mapping if you skip the cascade:
- `Rollup failed to resolve import "marked" from "shared/markdown.ts"` — root install missing.
- `'vite' is not recognized as an internal or external command` during the mobile build — mobile install missing.

## Running in development

```bash
# Live reload — frontend via Vite, Go via Wails' rebuild watcher
wails dev
```

Frontend hot-reload is instant. Go changes trigger an automatic backend rebuild.

## Building a release binary

```bash
wails build
```

Output lands in `build/bin/`. The result is a single-file Windows executable with no external runtime dependencies.

## Running tests

```bash
# Go unit tests
go test ./...

# Frontend type and a11y check
cd frontend && npx svelte-check
```

Both must be green before a change lands. The Go test suite covers the repository layer, LLM tool plumbing, agent scheduler, importer, and indexer. `svelte-check` catches type errors and a11y regressions in the Svelte components.

> **Note:** Two `//go:embed` directives need bundle output to exist before `go build ./...` works on a fresh checkout — `main.go` embeds `frontend/dist` (the desktop UI) and `mobile/embed.go` embeds `mobile/dist` (the mobile PWA). Without them you'll see `pattern all:<dir>: no matching files found`. Build them via `cd frontend && npm install && npm run build` and `cd mobile && npm install && npm run build` (or run `wails dev` / `wails build` for the frontend, which builds it as a side effect). Once both `dist/` directories exist, plain `go build` and `go test` work normally.

## Project coding standards

See [CLAUDE.md](CLAUDE.md) for the full list — those standards were written for AI collaboration but apply to every contributor. The short version:

- **All user-facing strings are localised.** Never hardcode display text.
- **No `any` in TypeScript.** Use proper interfaces, unions, or generics.
- **No native `confirm()` or `alert()`.** Use the in-app `ConfirmDialog` and toast system.
- **Components stay under ~300 lines.** If a component grows past that, extract sub-concerns.
- **ID-based state, not index-based.** Never key mutable state by array index.
- **Extract reusable patterns.** Svelte actions for DOM behaviours, stores for shared state, components for repeated UI.
- **No dead code.** Remove unused imports, dead branches, and redundant logic proactively.
- **Drag-and-drop wherever it makes sense**, not up/down buttons.

## Architecture

### Directory layout

```
bruv-1.0/
├── main.go              # Wails app entry point (desktop, --server, and service modes)
├── app.go               # App struct — desktop-only concerns: window, tray, local HTTP transport
├── shell_bridge.go      # ShellAPI — the narrow Wails-bound surface (dialogs, shell-open, pairing bootstrap)
├── service.go           # Windows Service install/uninstall/run (kardianos/service)
├── wails.json           # Wails project config
├── cmd/
│   └── bruv-server/     # Headless backend binary (same service layer, no GUI)
├── core/                # Domain layer — shared by desktop and server
│   ├── capture/         # URL → structured clip resolvers (Twitter/Truth Social/Reddit/YouTube)
│   ├── events/          # In-memory event bus
│   ├── reposync/        # Filesystem watcher → change events
│   ├── runtime/         # LLM runtime: chat, agents, tool dispatch, prompts, MCP bridging
│   ├── services/        # Card, project, catalog, search, chat, agent, workspace services
│   ├── supervisor/      # Multi-repo Runtime host; the per-repo JSON-RPC method surface
│   └── workspace/       # Path-safety chokepoint for on-disk workspaces
├── transport/
│   └── http/            # JSON-RPC 2.0 + SSE + auth + static hosting + /present output page
├── internal/            # Infrastructure — no domain logic
│   ├── agent/ config/ importer/ index/ llm/ logging/ mcp/ mcpserver/ model/
│   ├── notify/ push/ repo/ repocli/ schema/ server/ update/ workspace/
│   └──                  # (repo/ owns the portable on-disk format; model/ the data types)
├── foldertemplate/      # Card/workspace folder templates (its own Go module)
├── frontend/            # Desktop UI (Svelte 5 + Wails)
│   └── src/
│       ├── components/  # Svelte components
│       ├── lib/         # Stores, actions, locales, the test mock adapter
│       └── UI-CONVENTIONS.md   # The UI contract — read before adding shared components
├── mobile/              # Phone PWA, served at /m/ (embedded into the binary at build time)
├── shared/              # TypeScript shared by both surfaces (@shared/ alias): types, api.ts, the backend adapter
├── clipper/             # Web clipper browser extension (Chrome MV3, sideloaded)
├── docs/                # User-facing docs (self-hosting, MCP)
├── scripts/             # Dev/deploy helpers
├── website/             # Landing site (GitHub Pages → bruv-ai.app)
├── plan/                # Project journal + TODO (gitignored)
└── build/               # Build assets (icons, Wails platform configs, NSIS installer)
```

### Repo format contract

BRUV repos are designed to be self-contained and portable. The format is stable from `v1.0` final onward — any future additions must preserve this invariant: **the repo folder contains everything needed to render the project, and nothing personal to the user who created it**.

```
<repo>/
├── manifest.json            # repo metadata incl. stable UUID `id`
├── card_types.json          # user-defined types, templates, builtin overrides
├── tags.json                # repo-global tag color cache (cross-project consistency)
├── mcp_servers.json         # MCP server definitions (secrets in OS keychain, not here)
├── capture_prefs.json       # capture defaults (shared by every device capturing into this repo)
├── template_prefs.json      # slide-template matching priorities + urlHint overrides
├── activity/<actorID>.jsonl # per-actor activity log shards (one file per writer)
├── attachments/<card-id>/   # card attachment files
├── brands/                  # hierarchy root
│   └── <brand-slug>/
│       ├── brand.json
│       └── streams/
│           └── <stream-slug>/
│               ├── stream.json
│               └── projects/
│                   └── <project-slug>/
│                       ├── project.json
│                       ├── tags.json                  # per-project tag definitions
│                       ├── members.json               # project members (optional)
│                       ├── workspace/                 # workspace.json + index.json (optional)
│                       └── categories/
│                           └── <cat-slug>.json
├── cards/
│   ├── <card-id>.json           # card content + blocks
│   ├── <card-id>.agent.json     # agent config (optional; run history lives in <configDir>/runs/)
│   └── <card-id>.comments.json  # comments (optional)
├── pins/
│   └── <card-id>/pins.json      # cross-project pinning
├── types/                        # optional community schema drops
└── .bruv/                        # PRIVATE — gitignored, derived state only
    ├── index.db                  # SQLite FTS index (rebuildable)
    └── instance.lock             # one BRUV process per repo folder (held while the repo is open)
```

`.bruv/instance.lock` is taken when a runtime opens the repo ([core/supervisor/runtime.go](core/supervisor/runtime.go)). A second BRUV process opening the same folder — say the desktop app opening the folder the local BRUV Server already serves — fails with an error naming the holder; it should connect to that server instead. File-syncing a live repo folder between machines (Syncthing, Dropbox) is not supported for the same reason: each machine would run its own agents and writers against the same files.

**Personal state lives in the OS config folder**, split into two zones:

```
<configDir>/                       # server-owned: shared by every device pointed at this server
├── chats/<repoID>/<chatID>.messages.json
├── runs/<repoID>/<cardID>.json    # agent run history (kept out of the repo so it never travels)
├── repos.json                     # the repo registry this backend serves
├── llm_accounts.json              # metadata only; API keys in OS keychain
├── llm_config.json                # mode + system context
├── llm_routing.json               # models, routers, per-task assignments
├── notifications.json
├── notify_config.json             # SMTP/webhook destinations
├── preferences.json               # server zone: default category name, due-date notify config, importer creds
├── profile.json                   # display name, role, bio (per-user, not per-device)
├── pricing.json                   # optional hand-edited token-pricing overrides
├── devices.json, bootstrap-token.txt, secret.key   # enrolled devices, enrolment seed, URL-signing key
├── vapid.json, push-subscriptions.json             # Web Push keypair + phone subscriptions
├── crashes/, logs/                # operational state
└── clientdata/                    # CLIENT-owned: per-device, never follows the user/server
    ├── connections.json           # known remote BRUV servers + active pointer
    ├── device-id.txt              # stable per-device UUID (activity-log shard key)
    ├── device-token.txt           # this device's bearer token for the local server
    ├── repo-recents.json          # recently-opened repos per connection
    ├── workspace-checkouts.json   # where this device cloned workspaces
    ├── ui_preferences.json        # per-device UI prefs: theme, locale, layout, first-run flags
    └── window.json                # window bounds
```

**The contract:**

1. **Never write personal state into the repo folder.** If a new feature needs per-user, per-machine state, it goes in the config folder keyed by `repoID`. This is what makes sharing work.
2. **Never assume the config folder follows the repo.** Shared repos land on a machine with empty chat history, zero notifications, and whatever LLM accounts the new user has configured.
3. **Repo IDs are stable across machines.** Alice's repo zipped to Bob has the same `manifest.json` → `id` on both sides. The ID is a keying convenience, not a secret.
4. **Server vs. client zones are physical.** Once Mode A/B remote-server deployments separate the host from the client device, *only* `clientdata/` follows the desktop app — everything outside it lives on the server.

#### Server vs. client placement audit

When adding a persistence surface, ask:

| Question | If yes → | If no → |
|---|---|---|
| Would two devices pointed at the same server want to see the same value? | server (`<configDir>/`) | client (`<configDir>/clientdata/`) |
| Does this represent the human user (identity, preferences, content)? | server | — |
| Does this represent this physical device (window bounds, what filesystem paths exist here)? | — | client |

Status of existing files:

| File | Zone | Notes |
|---|---|---|
| `chats/`, `runs/`, `repos.json`, `llm_accounts.json`, `llm_config.json`, `llm_routing.json`, `notify_config.json`, `notifications.json`, `pricing.json`, `profile.json`, `devices.json`, `vapid.json`, `push-subscriptions.json`, `crashes/`, `logs/` | ✅ server | Shared identity + content; correct. |
| `clientdata/connections.json`, `device-id.txt`, `device-token.txt`, `repo-recents.json`, `workspace-checkouts.json`, `window.json` | ✅ client | Per-device by definition; correct. |
| `preferences.json` | ✅ server | Split completed 2026-06-13: holds only server-zone fields (default category name, due-date notification config, Trello importer credentials). Reached over RPC (`GetPreferences`/`SetPreferences`). |
| `clientdata/ui_preferences.json` | ✅ client | Per-device UI prefs (theme, locale, sidebar width/collapse, type-badge display, inbox limits, reopen-last-repo, LLM-nudge-shown, local-server-port). Served by the local shell (`ShellAPI.Get/SetUIPreferences`) in every desktop mode — never over RPC; browser mode falls back to localStorage. One-shot read-time migration lifts legacy fields from `preferences.json` on first load. `local_server_port` (added 2026-07-25) overrides the embedded HTTP server's loopback port so URL-pairing tools (web clipper) survive restarts. 0 = the default: 9870, read at boot in `startHTTPTransport`; if the requested port is taken it walks 9870–9879, then falls back to an ephemeral port, and the UI reports the port it actually got. |

When adding a new persistence surface, ask: *"If Alice shares this repo with Bob, should Bob see this?"* If yes, it goes in the repo. If no, it goes in the config folder, then ask the device-vs-server question to pick the zone.

### Backend adapter architecture

Every surface talks to the Go backend over the same HTTP transport ([transport/http/](transport/http/)): JSON-RPC 2.0 for calls, Server-Sent Events for live updates. The desktop app is no exception — it runs a loopback server in-process and its UI is a client of it, exactly like a phone or a second desktop connected to a remote BRUV Server.

```
UI components  →  shared/api.ts  →  getBackend()  →  shared/adapters/cloud.ts  →  HTTP
                                                                                  ├─ /repos/<id>/rpc   → supervisor.Runtime        (per repo)
                                                                                  ├─ /server/rpc       → supervisor.MachineService (per machine)
                                                                                  └─ Wails ShellAPI    → shell_bridge.go           (desktop shell only)
```

- **[shared/types.ts](shared/types.ts)** — the `BackendAdapter` interface (every method the desktop UI can call) plus the shared data types.
- **[shared/api.ts](shared/api.ts)** — one export per method, delegating to `getBackend()`. Components import from `@shared/api` and never touch an adapter.
- **[shared/adapters/index.ts](shared/adapters/index.ts)** — `initBackend()` / `getBackend()`. There is one production adapter; `setBackend()` lets tests install `frontend/src/lib/adapters/mock.ts`.
- **[shared/adapters/cloud.ts](shared/adapters/cloud.ts)** — the adapter. A proxy turns any method name into a JSON-RPC call: names in `SERVER_METHODS` go to `/server/rpc`, everything else to the active repo's `/repos/<id>/rpc`. Names in `SHELL_METHODS` (native dialogs, opening folders, workspace checkouts on this device, build info, update check) are never sent over RPC — they call the Wails-bound `ShellAPI` in [shell_bridge.go](shell_bridge.go) and fail with a clear error when there is no desktop shell. Per-device UI preferences (`Get/SetUIPreferences`) are also local: the shell, or `localStorage` in a browser.
- **Mobile** ([mobile/](mobile/)) doesn't use the adapter: it calls `repoRPC(method, params)` / `machineRPC(method, params)` from [mobile/src/lib/auth.ts](mobile/src/lib/auth.ts). The **clipper** has its own `repoRPC` in [clipper/src/lib/api.ts](clipper/src/lib/api.ts).
- **Go side.** The transport dispatches by reflection, but only to the names a target lists in its `RPCMethods()`. Those lists live in [core/supervisor/rpc_surface.go](core/supervisor/rpc_surface.go): `repoRPCMethods` for `*Runtime`, `machineRPCMethods` for `*MachineService`. Any other exported method is unreachable over the network.
- **Events.** The backend publishes on an event bus; clients subscribe over SSE (`subscribe(cb)` / `unsubscribe(cb)` on the adapter). A new topic must be added to `KNOWN_TOPICS` in [shared/adapters/topics.ts](shared/adapters/topics.ts), or no subscriber receives it.

#### Adding a backend method

1. **Implement it in Go.** Per-repo behaviour goes on `*supervisor.Runtime` (usually a thin method in [core/supervisor/runtime_methods.go](core/supervisor/runtime_methods.go) delegating to a service in `core/services/`). Per-machine behaviour — settings, profile, LLM accounts, anything that works before a repo is picked — goes on `*supervisor.MachineService` in [core/supervisor/machine.go](core/supervisor/machine.go). Arguments are positional and JSON-decoded, so keep them to JSON-friendly types. A method that can only run in the desktop process (a native dialog, a file on this device) goes on `ShellAPI` in [shell_bridge.go](shell_bridge.go) instead.
2. **Classify it in [core/supervisor/rpc_surface.go](core/supervisor/rpc_surface.go).** Add the name to `repoRPCMethods` or `machineRPCMethods`, or — if it's internal plumbing that must not be network-callable — to the internal lists in [rpc_surface_test.go](core/supervisor/rpc_surface_test.go). `TestRPCSurfacePinned` fails until every exported method is classified exactly once.
3. **Declare it in [shared/types.ts](shared/types.ts)** on `BackendAdapter`, and add a stub to [frontend/src/lib/adapters/mock.ts](frontend/src/lib/adapters/mock.ts) so the test adapter still type-checks.
4. **Export it from [shared/api.ts](shared/api.ts).**
5. **Route it in [shared/adapters/cloud.ts](shared/adapters/cloud.ts)** when it isn't a per-repo call: add a `MachineService` method to `SERVER_METHODS`, a `ShellAPI` method to `SHELL_METHODS`. Per-repo methods need no entry.
6. **Call it.** Desktop components import it from `@shared/api`; mobile calls `repoRPC('Name', [args])` or `machineRPC('Name', [args])`.

A method missing from step 2 answers "method not found" over RPC; a machine method missing from step 5 is sent to the repo endpoint and fails before a repo is picked.

#### Identity model

| Concept | Purpose | Local behaviour |
|---|---|---|
| **UserProfile** | Editable display identity (name, role, bio, expertise, avatar) | Auto-populates display name from the OS account on first launch |
| **AuthInfo** | Authentication state (id, provider, email, authenticated) | Returns the local OS username, `provider: "local"`, `authenticated: true` |
| **LLMConfig** | AI-specific settings (system prompt, etc.) | Persisted to `llm_config.json` in the config directory |

#### Capabilities

`getCapabilities()` returns a `BackendCapabilities` object that UI components check before rendering local-only features:

```ts
type BackendCapabilities = {
  hasLocalFilesystem: boolean  // true inside the desktop shell: folder pickers, path inputs, open-in-Explorer
  hasAuth: boolean             // login/logout flows (not used yet; devices enrol with a token)
  hasRealtime: boolean         // live event subscriptions (SSE)
}
```

## Filing bugs

Open an issue on GitHub with:

- What you did
- What you expected
- What actually happened
- BRUV version (shown in the About dialog)
- Windows version
- Anything interesting from the log folder (**About → Open log folder**)

## License

By contributing, you agree your contributions are licensed under the [MIT License](LICENSE).
