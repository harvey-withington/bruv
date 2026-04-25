# Privacy

BRUV is **local-first**. Your data lives on your machine, in files you own, in formats you can read. This document explains exactly what that means in practice — what stays on disk, what goes over the network, and what the AI agents can and can't do.

## What stays on your machine

Everything you create in BRUV is stored as plain JSON files on your local disk, split across two locations:

### Your repo folder (shareable)

Whatever path you chose when you created a repo. This folder contains the project itself — brands, streams, projects, categories, cards, tags, agent configs, card types and templates. It is deliberately designed to be **self-contained and portable**: zip it, commit it to git, drop it on a USB stick, and everything the project needs travels with it. See [README.md](README.md#sharing-a-repo) for the sharing story.

```
<your-repo>/
├── .bruv/
│   ├── manifest.json      # repo metadata (name, stable ID, description)
│   └── card_types.json    # your custom card types + templates for this repo
├── brands/                # hierarchy: brands → streams → projects → categories
├── cards/
│   ├── <id>.json          # card content
│   └── <id>.agent.json    # agent config + run history (if the card has one)
├── pins/
│   └── <id>/pins.json     # cross-project card pinning
└── types/                 # optional: community card type schema drops
```

### Your config folder (personal, machine-local)

**Location (Windows):** `%APPDATA%\bruv\`

Open from **BRUV → About → Open config folder**. This folder contains per-user, per-machine state that should **not** travel when you share a repo:

- `llm_accounts.json` — AI provider metadata (API keys live in your OS keychain, see below)
- `llm_config.json` — LLM system prompt and related settings
- `notify_config.json` — notification channel preferences
- `preferences.json` — UI preferences, theme, locale, etc.
- `profile.json` — display name, avatar
- `notifications.json` — notification history
- `pricing.json` — token pricing table
- `window.json` — remembered window size and position
- `recent.json` — list of recently opened repo paths
- **`chats/<repoID>/`** — AI chat history, keyed by repo ID so that shared repos don't leak personal conversations

**There is no cloud sync, no account, no login.** If you delete the config folder, you've reset BRUV to first-run state but your repo is untouched. If you delete a repo folder, only that repo is gone.

## What goes over the network

BRUV itself makes no network calls. The only outbound traffic happens when:

1. **You run an AI agent or use AI chat.** Your prompt, the card context, and whatever tool results the agent generates are sent to the LLM provider you configured — and only that provider. BRUV supports:
   - **Anthropic** → `https://api.anthropic.com`
   - **OpenAI** → `https://api.openai.com` (or a custom base URL you set, if you're using an OpenAI-compatible endpoint)
   - **Ollama** → `http://localhost:11434` by default — fully local, nothing leaves your machine
2. **An agent uses a web tool.** If you grant an agent the `web_search`, `web_fetch`, or `http_request` tool, it can:
   - Query DuckDuckGo via `https://html.duckduckgo.com/html/` (`web_search`)
   - Fetch arbitrary URLs you or the agent specify (`web_fetch`, `http_request`)
3. **You configure an MCP server.** External Model Context Protocol servers run as local subprocesses on your machine and may make their own network calls depending on what the server does — e.g. a GitHub MCP server calls github.com, a Kiwi flight MCP server calls Kiwi's API. These calls come from the MCP server, not BRUV. You control which servers to install; each server's source code and network behaviour is the user's responsibility to vet. See [docs/mcp-servers.md](docs/mcp-servers.md) for the full security model.

That's the complete list. There is:

- **No telemetry**
- **No analytics**
- **No crash reporting**
- **No update pings** (for now — see [README](README.md) for the auto-update story)
- **No advertising, tracking, or third-party scripts**

If you never configure an LLM account and never use AI features, BRUV makes zero network requests.

### Self-hosted server mode (optional)

If you opt into running BRUV as a server (the **Server** checkbox in the installer — see [docs/self-hosting.md](docs/self-hosting.md)), the server listens on `0.0.0.0:9870` and accepts connections from devices that have enrolled with a per-device token. Bytes that flow:

- **JSON-RPC + SSE** between client devices and the server, carrying card data and event notifications.
- **Attachment downloads** via short-lived, HMAC-signed URLs (`/attachments/<cardID>/<id>?exp=&sig=`). The signing secret lives in the server's config dir (`%APPDATA%\bruv\secret.key`), is generated once on first server start, and never leaves the server. URL TTL is 5 minutes — a leaked URL stops working very quickly.
- **No data is sent to BRUV's authors**, ever. The server is yours; we have no relationship with it.

The transport is plain HTTP and is intended to be reached over Tailscale (or any private network). BRUV does not bundle TLS termination — Tailscale-serve will wrap the server in HTTPS for free if you want a public-on-the-tailnet URL, but the BRUV process itself stays HTTP-only and only listens on the addresses you tell it to.

## What the AI agents can access

Agents can only use tools you've explicitly enabled for each card. The full tool set:

| Tool | What it does | Scope |
|---|---|---|
| `web_search` | Searches DuckDuckGo | Public web |
| `web_fetch` | Fetches a specific URL | Public web |
| `http_request` | Makes an HTTP request (GET/POST/PUT/DELETE) | Public web |
| `notify` | Sends you a desktop / in-app notification | Local |
| `update_self` | Updates blocks on the card it's attached to | Local, scoped to one card |
| `read_card` | Reads another card's content | Local, scoped to the current project |
| `create_card` | Creates a new card | Local, scoped to the current project |

**Agents cannot:**

- Read files outside BRUV's config directory
- Execute shell commands or scripts
- Access other applications, your browser, or your clipboard
- Reach cards in projects other than the one they belong to (scope is enforced in code)
- Modify BRUV's own configuration files

The **Tool Permissions** panel in each agent card lets you enable or disable each tool individually. An agent with no tools enabled can still chat, but can't take action.

## Your API keys

When you add an LLM account, your API key is stored in the **OS keychain** — Windows Credential Manager on Windows, Keychain on macOS, or libsecret on Linux. The `llm_accounts.json` file on disk stores only the non-secret metadata (provider, model, label) with the `api_key` field blank.

- Keys never touch the disk in plaintext on systems with a working keychain.
- Keys never leave your machine except in the `Authorization` header of requests to the provider you configured them for.
- You can see BRUV's stored secrets in your OS keychain viewer under the service name **BRUV**.
- If the OS keychain is unavailable (broken libsecret daemon, locked-down corporate machine), BRUV transparently falls back to storing keys in `llm_accounts.json` with user-only file permissions, exactly as earlier versions did. The goal is to upgrade security when possible, never to lock you out of your own data.
- If you share your config directory (backup, sync tool, etc.), you are **not** sharing your API keys on a system with a working keychain — the JSON file has nothing sensitive in it. On a fallback-to-plaintext system, you are sharing the keys; be aware.

If you'd rather not store keys anywhere at all, configure **Ollama** instead and run models locally.

### Migrating from earlier versions

If you're upgrading from a BRUV build that predates Sprint B (the keychain backend), the first launch will automatically move any plaintext API keys out of `llm_accounts.json` and into the OS keychain. The migration is one-way and idempotent — it only rewrites the JSON file if there are plaintext keys left to migrate.

## Sharing a repo

BRUV repos are designed to be shared. The repo folder contains everything a project needs — cards, hierarchy, tags, agent configs, card types and templates — and nothing personal. When you share a repo (zip, git, cloud sync, USB), only the project travels; your AI chats, API keys, and settings stay on your machine.

**What does NOT travel with a shared repo:**

- AI chat history (stored per-user in your config folder, keyed by repo ID)
- LLM API keys (OS keychain)
- Notification history, preferences, profile, window state
- Agent run history from YOUR copy (configs travel; the history of what your agents have actually done stays local to each machine)

**What DOES travel with a shared repo:**

- Cards, tags, brands, streams, projects, categories
- Card types and templates defined in this repo
- Agent configurations (schedules, tools, budgets, safety rails)
- Attachments and comments
- **MCP server definitions** (`.bruv/mcp_servers.json`) — the command, args, and list of env var *names* each server needs. The recipient sees which servers to install but has to provide their own API keys via the MCP Servers dialog.

**What does NOT travel with shared MCP configs:**

- API keys or other env var values — these are per-user per-machine via the OS keychain
- Server-side caches, logs, or auth tokens the subprocess might write to its own config dir

When someone else opens your shared repo, they get a fresh chat history and an empty notification inbox for that repo — their usage stays separate from yours.

### Before sharing

If you've been running agents in your copy of a repo, the `.agent.json` files contain run history that includes token counts, timestamps, and any recorded outputs. This is not sensitive in most cases but you may want to clear it before sharing. You can do this today by opening each agent card → **Clear run history**. A one-click "prepare for sharing" sweep is tracked in the backlog for a post-v1.0a release.

## How to wipe everything

Close BRUV, delete `%APPDATA%\bruv\`, restart BRUV. You're back to first-run.

## Questions or concerns

Open an issue on the GitHub repository (linked from the About dialog) or check the source yourself — every network call in BRUV is in [internal/llm/](internal/llm/) or [internal/agent/web.go](internal/agent/web.go). There's nothing hidden.
