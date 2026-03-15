# BRUV

> Your most organised mate.

An AI-native, local-first productivity app combining the structure of a kanban board with the intelligence of an LLM assistant. Built by Good Egg Software.

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
