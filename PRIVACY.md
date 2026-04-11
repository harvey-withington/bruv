# Privacy

BRUV is **local-first**. Your data lives on your machine, in files you own, in formats you can read. This document explains exactly what that means in practice — what stays on disk, what goes over the network, and what the AI agents can and can't do.

## What stays on your machine

Everything you create in BRUV — brands, streams, projects, cards, tags, agents, chat history, notifications — is stored as plain JSON files on your local disk.

**Location (Windows):** `%APPDATA%\bruv\`

You can open this folder from **BRUV → About → Open config folder** (or navigate to it manually). Inside you'll find:

- Your project data (the repository itself)
- `llm_accounts.json` — your configured AI providers and API keys (see note below)
- `llm_config.json` — LLM system prompt and related settings
- `notify_config.json` — notification channel preferences
- Chat history files, one per card or project chat
- `window.json` — remembered window size and position

**There is no cloud sync, no account, no login.** If you delete this folder, you've deleted BRUV's knowledge of everything.

## What goes over the network

BRUV itself makes no network calls. The only outbound traffic happens when:

1. **You run an AI agent or use AI chat.** Your prompt, the card context, and whatever tool results the agent generates are sent to the LLM provider you configured — and only that provider. BRUV supports:
   - **Anthropic** → `https://api.anthropic.com`
   - **OpenAI** → `https://api.openai.com` (or a custom base URL you set, if you're using an OpenAI-compatible endpoint)
   - **Ollama** → `http://localhost:11434` by default — fully local, nothing leaves your machine
2. **An agent uses a web tool.** If you grant an agent the `web_search`, `web_fetch`, or `http_request` tool, it can:
   - Query DuckDuckGo via `https://html.duckduckgo.com/html/` (`web_search`)
   - Fetch arbitrary URLs you or the agent specify (`web_fetch`, `http_request`)

That's the complete list. There is:

- **No telemetry**
- **No analytics**
- **No crash reporting**
- **No update pings** (for now — see [README](README.md) for the auto-update story)
- **No advertising, tracking, or third-party scripts**

If you never configure an LLM account and never use AI features, BRUV makes zero network requests.

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

## How to wipe everything

Close BRUV, delete `%APPDATA%\bruv\`, restart BRUV. You're back to first-run.

## Questions or concerns

Open an issue on the GitHub repository (linked from the About dialog) or check the source yourself — every network call in BRUV is in [internal/llm/](internal/llm/) or [internal/agent/web.go](internal/agent/web.go). There's nothing hidden.
