<p align="center">
  <img src="frontend/src/assets/images/bruv-icon.svg" alt="BRUV logo" width="128" />
</p>

# BRUV

> Your most organised best bud.

An AI-native, local-first productivity app combining the structure of a kanban board with the intelligence of an LLM assistant. Built with :gift_heart: by Harvey & :robot: Claude.

## Tech Stack

- **Desktop shell:** [Wails v2](https://wails.io/) (Go backend + web frontend)
- **Frontend:** [Svelte 5](https://svelte.dev/) + TypeScript + Vite
- **Backend:** Go — repository I/O, SQLite indexing, MCP server
- **LLM:** Provider-agnostic (Anthropic, OpenAI, Ollama)

## Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/)
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)

## Development

```bash
# Run in live development mode (hot reload)
wails dev
```

The frontend dev server provides hot reload for Svelte changes. Go changes trigger a rebuild automatically.

## Building

```bash
# Build a production binary
wails build
```

Output lands in `build/bin/`.

## oDrive Sync Reminder

If you use oDrive to sync this repo, install the bundled VS Code extension that reminds you to resume sync when closing the editor:

```powershell
# For Windsurf:
cmd /c mklink /J "%USERPROFILE%\.windsurf\extensions\odrive-reminder" "tools\odrive-reminder"

# For VS Code:
cmd /c mklink /J "%USERPROFILE%\.vscode\extensions\odrive-reminder" "tools\odrive-reminder"
```

Restart VS Code after creating the junction.

## Backend Adapter Architecture

The frontend is decoupled from the Wails/Go backend via an adapter pattern, making it possible to swap in a cloud or SaaS backend without changing any UI components.

### How it works

```
UI Components  →  api.ts (delegation)  →  getBackend()  →  adapter (wails / cloud / …)
```

- **`src/lib/types.ts`** — defines the `BackendAdapter` interface (every method the UI can call) plus shared types (`UserProfile`, `AuthInfo`, `LLMConfig`, `BackendCapabilities`).
- **`src/lib/adapters/wails.ts`** — the local adapter; thin wrapper around auto-generated Wails Go bindings.
- **`src/lib/adapters/index.ts`** — reads `VITE_BACKEND` env var (`"wails"` by default) and lazily loads the matching adapter.
- **`src/lib/api.ts`** — re-exports every method via `getBackend()`, so components import from `api.ts` and never reference an adapter directly.

### Implementing a new backend

1. **Create `src/lib/adapters/mybackend.ts`** exporting a `BackendAdapter` object. Every method in the interface must be implemented — use the Wails adapter as a reference.
2. **Register it** in `src/lib/adapters/index.ts` by adding a `case` to the `switch`:
   ```ts
   case 'mybackend': {
     const { myAdapter } = await import('./mybackend')
     _adapter = myAdapter
     break
   }
   ```
3. **Set the env var** `VITE_BACKEND=mybackend` (or add it to a `.env` file in `frontend/`).
4. **Return appropriate capabilities** from `getCapabilities()` — the UI uses these to show/hide local-only features (e.g. folder picker, file path inputs).

### Identity model

The adapter exposes three separate concerns:

| Concept | Purpose | Local behaviour |
|---------|---------|-----------------|
| **UserProfile** | Editable display identity (name, role, bio, expertise, avatar) | Auto-populates display name from Windows account on first launch |
| **AuthInfo** | Authentication state (id, provider, email, authenticated) | Returns local OS username, `provider: "local"`, `authenticated: true` |
| **LLMConfig** | AI-specific settings (context prompt, etc.) | Persisted to `llm_config.json` in the app config directory |

### Capabilities

`getCapabilities()` returns a `BackendCapabilities` object that UI components check before rendering local-only features:

```ts
interface BackendCapabilities {
  hasLocalFilesystem: boolean  // folder picker, path inputs
  hasAuth: boolean             // login/logout flows
  hasRealtime: boolean         // live event subscriptions
}
```

### Events

The adapter supports `subscribe(cb)` / `unsubscribe(cb)` for real-time push events. The local Wails adapter currently no-ops these (Wails uses its own event system), but a cloud adapter would use WebSockets or SSE to push `BackendEvent` objects to subscribers.

## Project Structure

```
bruv-1.0/
├── main.go              # Wails app entry point
├── app.go               # App struct — Go methods exposed to frontend
├── wails.json           # Wails project config
├── frontend/
│   ├── src/             # Svelte 5 app source
│   └── wailsjs/         # Auto-generated Go bindings (gitignored)
└── build/               # Build assets (icons, platform configs)
```

## License

Open source — license TBD.
