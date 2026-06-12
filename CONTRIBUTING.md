# Contributing to BRUV

Thanks for taking an interest. BRUV is a small, single-maintainer project built in the open, and contributions — code, bug reports, feedback — are welcome.

## Code of conduct

Be kind. Assume good faith. Disagree with ideas, not people. That's the whole thing.

## Tech stack

- **Desktop shell:** [Wails v2](https://wails.io/) (Go backend + web frontend, single native binary)
- **Frontend:** [Svelte 5](https://svelte.dev/) with runes + TypeScript + Vite
- **Backend:** Go — repository I/O, SQLite indexing, LLM provider adapters, agent runtime
- **LLM:** provider-agnostic (Anthropic, OpenAI, Ollama — more welcome)
- **Storage:** plain JSON files in the OS config directory (`%APPDATA%\bruv\` on Windows)

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
├── main.go              # Wails app entry point
├── app.go               # App struct — Go methods exposed to frontend
├── app_agent.go         # Agent-related methods on the App struct
├── tray_windows.go      # System tray (Windows); tray_other.go stubs Mac/Linux
├── wails.json           # Wails project config
├── internal/
│   ├── agent/           # Agent runtime, scheduler, due-date scanner, web tools
│   ├── config/          # User config + personal state (LLM accounts, chats, prefs)
│   ├── importer/        # Trello JSON importer
│   ├── index/           # SQLite full-text search index
│   ├── llm/             # Provider adapters (Anthropic, OpenAI, Ollama), tool definitions
│   ├── model/           # Shared data model (Brand, Stream, Project, Card, Block, ...)
│   ├── notify/          # Notification dispatcher (in-app, system, email, webhook)
│   ├── repo/            # Repository layer — atomic JSON file IO, portable repo format
│   ├── schema/          # Card type JSON schema system
│   └── update/          # GitHub Releases update checker
├── frontend/
│   └── src/
│       ├── components/  # Svelte 5 components
│       ├── lib/         # Stores, actions, adapters, API surface
│       └── assets/      # Icons, fonts, images
└── build/               # Build assets (icons, Wails platform configs, NSIS installer)
```

### Repo format contract

BRUV repos are designed to be self-contained and portable. The format is stable from v1.0a onward — any future additions must preserve this invariant: **the repo folder contains everything needed to render the project, and nothing personal to the user who created it**.

```
<repo>/
├── manifest.json            # repo metadata incl. stable UUID `id`
├── card_types.json          # user-defined types, templates, builtin overrides
├── tags.json                # repo-global tag color cache (cross-project consistency)
├── mcp_servers.json         # MCP server definitions (secrets in OS keychain, not here)
├── activity/<actorID>.jsonl # per-actor activity log shards (one file per writer)
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
│                       └── categories/
│                           └── <cat-slug>.json
├── cards/
│   ├── <card-id>.json           # card content + blocks
│   ├── <card-id>.agent.json     # agent config (optional)
│   └── <card-id>.comments.json  # comments (optional)
├── pins/
│   └── <card-id>/pins.json      # cross-project pinning
├── types/                        # optional community schema drops
└── .bruv/                        # PRIVATE — gitignored, derived state only
    ├── index.db                  # SQLite FTS index (rebuildable)
    └── lock                      # single-process lock file
```

**Personal state lives in the OS config folder**, split into two zones:

```
<configDir>/                       # server-owned: shared by every device pointed at this server
├── chats/<repoID>/<chatID>.messages.json
├── llm_accounts.json              # metadata only; API keys in OS keychain
├── llm_config.json                # mode + system context
├── notifications.json
├── notify_config.json             # SMTP/webhook destinations
├── preferences.json               # server zone: default category name, due-date notify config, importer creds
├── profile.json                   # display name, role, bio (per-user, not per-device)
├── pricing.json                   # cached LLM model pricing
├── card_types.json                # global default seeded into new repos
├── crashes/, logs/, runs/         # operational state
└── clientdata/                    # CLIENT-owned: per-device, never follows the user/server
    ├── connections.json           # known remote BRUV servers + active pointer
    ├── device-id.txt              # stable per-device UUID (activity-log shard key)
    ├── device-token.txt           # this device's bearer token for the local server
    ├── recent.json                # recently-opened repo paths
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
| `chats/`, `llm_accounts.json`, `llm_config.json`, `notify_config.json`, `notifications.json`, `pricing.json`, `card_types.json` (global default), `profile.json`, `crashes/`, `logs/`, `runs/` | ✅ server | Shared identity + content; correct. |
| `clientdata/connections.json`, `device-id.txt`, `device-token.txt`, `recent.json`, `window.json` | ✅ client | Per-device by definition; correct. |
| `preferences.json` | ✅ server | Split completed 2026-06-13: holds only server-zone fields (default category name, due-date notification config, Trello importer credentials). Reached over RPC (`GetPreferences`/`SetPreferences`). |
| `clientdata/ui_preferences.json` | ✅ client | Per-device UI prefs (theme, locale, sidebar width/collapse, type-badge display, inbox limits, reopen-last-repo, LLM-nudge-shown). Served by the local shell (`ShellAPI.Get/SetUIPreferences`) in every desktop mode — never over RPC; browser mode falls back to localStorage. One-shot read-time migration lifts legacy fields from `preferences.json` on first load. |

When adding a new persistence surface, ask: *"If Alice shares this repo with Bob, should Bob see this?"* If yes, it goes in the repo. If no, it goes in the config folder, then ask the device-vs-server question to pick the zone.

### Backend adapter architecture

The frontend is decoupled from the Wails/Go backend via an adapter pattern, making it possible to swap in a cloud or SaaS backend without touching any UI component.

```
UI Components  →  api.ts (delegation)  →  getBackend()  →  adapter (wails / cloud / …)
```

- **`src/lib/types.ts`** — defines the `BackendAdapter` interface (every method the UI can call) plus shared types (`UserProfile`, `AuthInfo`, `LLMConfig`, `BackendCapabilities`).
- **`src/lib/adapters/wails.ts`** — the local adapter; a thin wrapper around Wails' auto-generated Go bindings.
- **`src/lib/adapters/index.ts`** — reads the `VITE_BACKEND` env var (default `"wails"`) and lazily loads the matching adapter.
- **`src/lib/api.ts`** — re-exports every method via `getBackend()`, so components import from `api.ts` and never reference an adapter directly.

#### Implementing a new backend

1. Create `src/lib/adapters/mybackend.ts` exporting a `BackendAdapter` object. Every method in the interface must be implemented — use the Wails adapter as a reference.
2. Register it in `src/lib/adapters/index.ts` by adding a `case` to the switch:
   ```ts
   case 'mybackend': {
     const { myAdapter } = await import('./mybackend')
     _adapter = myAdapter
     break
   }
   ```
3. Set the env var `VITE_BACKEND=mybackend` (or add it to a `.env` file in `frontend/`).
4. Return appropriate capabilities from `getCapabilities()` — the UI uses these to show/hide local-only features (e.g. folder picker, file path inputs).

#### Identity model

The adapter exposes three separate concerns:

| Concept | Purpose | Local behaviour |
|---|---|---|
| **UserProfile** | Editable display identity (name, role, bio, expertise, avatar) | Auto-populates display name from the Windows account on first launch |
| **AuthInfo** | Authentication state (id, provider, email, authenticated) | Returns local OS username, `provider: "local"`, `authenticated: true` |
| **LLMConfig** | AI-specific settings (system prompt, etc.) | Persisted to `llm_config.json` in the app config directory |

#### Capabilities

`getCapabilities()` returns a `BackendCapabilities` object that UI components check before rendering local-only features:

```ts
interface BackendCapabilities {
  hasLocalFilesystem: boolean  // folder picker, path inputs
  hasAuth: boolean             // login/logout flows
  hasRealtime: boolean         // live event subscriptions
}
```

#### Events

The adapter supports `subscribe(cb)` / `unsubscribe(cb)` for real-time push events. The local Wails adapter currently no-ops these (Wails uses its own event system), but a cloud adapter would use WebSockets or SSE to push `BackendEvent` objects to subscribers.

## Adding a Wails-exposed Go method

Wails auto-generates TypeScript bindings from methods on the `App` struct. Adding a new method requires updating both the Go side and the adapter so the frontend can reach it:

1. Add the method to `app.go` (or `app_agent.go` for agent-related methods).
2. Keep the signature simple — Wails can marshal primitives, strings, maps, slices, and structs with JSON tags.
3. Run `wails dev` or `wails build` — bindings in `frontend/wailsjs/` regenerate automatically.
4. Add the method to the `BackendAdapter` interface in `src/lib/types.ts`.
5. Implement it in `src/lib/adapters/wails.ts` — just forward to the generated binding.
6. Export it from `src/lib/api.ts`.
7. Call it from your component via `api.yourMethod(...)`.

Skipping steps 4–6 causes silent "function is not a function" errors at runtime — the binding exists but the adapter doesn't know about it.

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
