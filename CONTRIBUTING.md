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
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)

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
├── tray.go              # System tray menu wiring
├── wails.json           # Wails project config
├── internal/
│   ├── agent/           # Agent runtime, scheduler, due-date scanner, web tools
│   ├── config/          # Config-dir IO: profile, LLM accounts, notify config, preferences
│   ├── importer/        # Trello JSON importer
│   ├── index/           # SQLite full-text search index
│   ├── llm/             # Provider adapters (Anthropic, OpenAI, Ollama), tool definitions
│   ├── model/           # Shared data model (Brand, Stream, Project, Card, Block, ...)
│   ├── notify/          # Notification dispatcher (in-app, system, email, webhook)
│   ├── repo/            # Repository layer — atomic JSON file IO
│   └── schema/          # Card type JSON schema system
├── frontend/
│   └── src/
│       ├── components/  # Svelte 5 components
│       ├── lib/         # Stores, actions, adapters, API surface
│       └── assets/      # Icons, fonts, images
└── build/               # Build assets (icons, Wails platform configs, NSIS installer)
```

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

## oDrive sync reminder (dev-machine only)

If you use oDrive to sync this repo on your dev machine, install the bundled VS Code extension that reminds you to resume sync when closing the editor:

```powershell
# For Windsurf:
cmd /c mklink /J "%USERPROFILE%\.windsurf\extensions\odrive-reminder" "tools\odrive-reminder"

# For VS Code:
cmd /c mklink /J "%USERPROFILE%\.vscode\extensions\odrive-reminder" "tools\odrive-reminder"
```

Restart the editor after creating the junction.

## License

By contributing, you agree your contributions are licensed under the [MIT License](LICENSE).
