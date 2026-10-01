# Privacy

BRUV is **local-first**. Your data lives on your machine, in files you own, in formats you can read. This document explains exactly what that means in practice — what stays on disk, what goes over the network, and what the AI agents can and can't do.

## What stays on your machine

Everything you create in BRUV is stored as plain files on your local disk, split across two locations:

### Your repo folder (shareable)

Whatever path you chose when you created a repo. This folder contains the project itself — brands, streams, projects, categories, cards, tags, agent configs, card types and templates. It is deliberately designed to be **self-contained and portable**: zip it, commit it to git, copy it to a USB stick, and everything the project needs travels with it. See [README.md](README.md#sharing-a-repo) for the sharing story.

```
<your-repo>/
├── manifest.json          # repo metadata (name, stable ID, description)
├── card_types.json        # your custom card types + templates for this repo
├── tags.json              # repo-wide tag colours
├── mcp_servers.json       # MCP server definitions (env var names only, never values)
├── capture_prefs.json     # capture defaults (video quality, size limits, where captures land)
├── template_prefs.json    # slide-template matching preferences
├── activity/              # activity log, one file per writer (who changed what, when)
├── attachments/<card-id>/ # files attached to cards
├── brands/                # hierarchy: brands → streams → projects → categories
│                          # (each project also holds its tags, members and any workspace config)
├── cards/
│   ├── <id>.json          # card content
│   ├── <id>.agent.json    # agent configuration (if the card has one)
│   └── <id>.comments.json # comments (if any)
├── pins/
│   └── <id>/pins.json     # where each card is pinned
├── types/                 # optional: community card type schema drops
└── .bruv/                 # derived, per-machine state — never shared
    ├── index.db           # SQLite search index (rebuilt from the files above)
    └── instance.lock      # lock file: only one BRUV process may open this folder at a time
```

BRUV writes a `.gitignore` that excludes `.bruv/` when it opens a repo (it leaves an existing one alone).

### Your config folder (personal, machine-local)

**Location (Windows):** `%APPDATA%\bruv\` for the desktop app. A BRUV Server installed as a Windows service uses `%PROGRAMDATA%\BRUV\` instead.

Open from **BRUV → About → Open config folder**. This folder contains per-user, per-machine state that should **not** travel when you share a repo:

- `llm_accounts.json` — AI provider metadata (API keys live in your OS keychain, see below)
- `llm_config.json`, `llm_routing.json` — AI behaviour settings, models and routing
- `notify_config.json` — notification channels (SMTP server details and webhook URL — see [Other credentials](#other-credentials))
- `notifications.json` — notification history
- `preferences.json` — settings shared by every device on this machine's backend: default category name, due-date reminder settings, Trello importer credentials
- `profile.json` — display name, avatar
- `repos.json` — the repos this machine knows about
- `mcp_approvals.json` — which repo MCP server commands you've approved to run on this machine
- `pricing.json` — optional token pricing overrides
- **`chats/<repoID>/`** — AI chat history, keyed by repo ID so that shared repos don't leak personal conversations
- **`runs/<repoID>/`** — agent run history (see [Before sharing](#before-sharing))
- `devices.json`, `bootstrap-token.txt`, `secret.key`, `vapid.json`, `push-subscriptions.json` — enrolled devices and the keys BRUV's local server uses to authenticate them, sign attachment links and register phones for push notifications
- `logs/` (kept for 7 days) and `crashes/`
- **`clientdata/`** — settings for this device only:
  - `ui_preferences.json` — theme, locale, sidebar and layout preferences, local server port, first-run flags
  - `window.json` — remembered window size and position
  - `connections.json`, `device-token.txt`, `device-id.txt` — the BRUV servers this device connects to and its token for them
  - `repo-recents.json` — recently opened repos
  - `workspace-checkouts.json` — where this device keeps local copies of workspaces

**There is no cloud sync, no account, no login.** If you delete the config folder, you've reset BRUV to first-run state but your repo is untouched. If you delete a repo folder, only that repo is gone.

## What goes over the network

BRUV sends nothing anywhere on its own: no telemetry, no analytics, no crash reporting, no background update checks. Every outbound connection below happens because you set something up or did something. Some run without a click once set up (a scheduled agent, a due-date email), and those are marked **automatic**.

1. **AI chat and agents.** When you send a chat message, run an agent, or use an AI action on a card (for example, **Create with AI**), the prompt, the card or project context, and the results of any tools used are sent to the LLM provider chosen for that task — and only that provider:
   - **Anthropic** → `https://api.anthropic.com`
   - **OpenAI** → `https://api.openai.com` (or a custom base URL you set, if you're using an OpenAI-compatible endpoint)
   - **Ollama** → `http://localhost:11434` by default — fully local, nothing leaves your machine

   Listing a provider's models and the **Test** button in AI settings also call that provider (with your key, but no card data). Scheduled agents call it **automatically** on their schedule.
2. **Web tools.** AI chat (in every mode) can use `web_search` and `web_fetch`. Agents can use them, plus `http_request`, only when you grant them:
   - `web_search` queries DuckDuckGo via `https://html.duckduckgo.com/html/`
   - `web_fetch` fetches a URL you or the model chose
   - `http_request` (agents only) sends a GET/POST/PUT/DELETE request, with an optional body, to a URL you or the model chose
3. **Capturing a post.** When you capture a link from Twitter/X, Truth Social, Reddit or YouTube into BRUV (for example by sharing it to the BRUV app on your phone), the BRUV backend reads the post from that platform's public endpoint — `cdn.syndication.twimg.com` (X), `truthsocial.com/api/v1/statuses/…`, `www.reddit.com/<post>.json`, or YouTube's oEmbed endpoint — then downloads the post's images, video and author avatar from wherever the platform serves them. No account or credentials are sent. A captured YouTube video plays through YouTube's embedded player, which loads from `youtube.com` when you view it; a video too large to store is kept as a link and streams from the platform when played.
4. **Importing from Trello.** BRUV reads the Trello export file you choose — it doesn't fetch the board itself. If the board has attachments, it downloads them (files stored on Trello through `api.trello.com`); when you've entered a Trello API key and token, they're sent with those downloads.
5. **Notifications.** Once you've configured them, notifications go out **automatically** (agent results, due-date reminders):
   - **Email** — to the SMTP server you set, with the notification's title and text.
   - **Webhook** — a JSON POST to the URL you set: title, text, source, card ID and card title, timestamp.
   - **Desktop notifications** are local.
   - **Web Push** — when you turn on push notifications on a phone, the phone's browser registers with its own push service (Google's, Apple's or Mozilla's, depending on the browser) and the BRUV Server stores that subscription. From then on, every in-app notification the server raises — agent results, due dates, alarms, pending clips — is sent, encrypted, through that push service to the phone: the notification's title, a short excerpt of its text, and a link to the card. Automatic once you opt in on the phone; turning push off on the phone removes the subscription. Only a BRUV Server sends pushes; the desktop shows its own system notifications.
6. **Checking for updates.** Only when you click **Check for updates** in the About dialog: one request to the GitHub Releases API (`api.github.com`).
7. **Connecting devices.** When you connect the desktop app, a phone, or the clipper to a BRUV Server, they exchange card data, events and attachments with **that server only** (see below). Cloning or syncing a workspace's local copy uses git over the same connection, only when you ask.
8. **MCP servers you install.** External Model Context Protocol servers run as local subprocesses on your machine; BRUV talks to them over stdin/stdout, not the network. They may make their own network calls depending on what the server does — e.g. a GitHub MCP server calls github.com. Those calls come from the MCP server, not BRUV. You control which servers to install; vetting each server's source code and network behaviour is up to you. See [docs/mcp-servers.md](docs/mcp-servers.md) for the full security model.

The **web clipper** browser extension talks to the BRUV server you pair it with (and checks it every 30 minutes for clips that still need finishing). When you capture a post, it also reads that post from the same platform endpoints as above, and downloads the post's media, from your browser.

There is:

- **No telemetry**
- **No analytics**
- **No crash reporting** (crash logs stay in your config folder)
- **No automatic update checks**
- **No advertising, tracking, or third-party scripts**

If you never configure an LLM account, notifications or a remote connection, and never capture, import or check for updates, BRUV makes zero outbound requests.

### Self-hosted server mode (optional)

If you opt into running BRUV as a server (the **Server** checkbox in the installer — see [docs/self-hosting.md](docs/self-hosting.md)), the server listens on `0.0.0.0:9870` and accepts connections from devices that have enrolled with a per-device token. (The desktop app's own built-in server listens on loopback only.) Bytes that flow:

- **JSON-RPC + SSE** between client devices and the server, carrying card data and event notifications.
- **Attachment downloads** via short-lived, HMAC-signed URLs (`/attachments/<cardID>/<id>?exp=&sig=`). The signing secret lives in the server's config folder (`secret.key`), is generated once on first server start, and never leaves the server. URL TTL is 5 minutes — a leaked URL stops working very quickly.
- **The MCP endpoint** (`/repos/<id>/mcp`), if you connect an external assistant — see [docs/mcp-server.md](docs/mcp-server.md).
- **No data is sent to BRUV's authors**, ever. The server is yours; we have no relationship with it.

The transport is plain HTTP and is intended to be reached over Tailscale (or any private network). BRUV does not bundle TLS termination — Tailscale-serve will wrap the server in HTTPS for free if you want a public-on-the-tailnet URL, but the BRUV process itself stays HTTP-only and only listens on the addresses you tell it to.

## What the AI agents can access

An agent can use **only the tools you grant it** on its card's Agent tab. Nothing is granted by default: an agent with no tools can only reply in text, and its run ends there. The Agent tab lists every tool an agent can be granted and ticks each one it has, so what you see is exactly what it can do.

| Tool | What it does | Reach |
|---|---|---|
| `web_search` | Searches DuckDuckGo | Public web |
| `web_fetch` | Fetches a specific URL | Public web |
| `http_request` | Makes an HTTP request (GET/POST/PUT/DELETE) | Public web |
| `notify` | Sends you a notification through your configured channels | Local, or your email / webhook |
| `update_self` | Updates the card the agent is attached to | That one card |
| BRUV board tools — `get_card`, `search_cards`, `list_cards`, `create_card`, `update_card`, `set_card_*`, `add_card_tags`, `remove_card_tags`, `create_card_type`, comments, attachments, pins, reading an agent's settings | The same board tools the in-app chat and the MCP connector use: read, create and change cards and their filing, and add a card type | Your whole board (this repo) |
| MCP server tools | Whatever the server you installed provides | Set by that server |

Board tools reach every card in the repo, not just the agent's own project — grant them only to agents you want working across your board.

**Agents cannot:**

- Start, stop, or reconfigure agents — including themselves (these tools can never be granted to an agent)
- Read or write files on your machine, except through an MCP server you have installed and granted
- Execute shell commands or scripts
- Access other applications, your browser, or your clipboard
- Modify BRUV's own configuration files

**Built-in limits on every run:** a token budget and a turn limit (both set per agent); an identical tool call is never repeated within a run; and a run stops early, marked failed, once its tool calls keep failing (for example, a site that blocks automated access). A run that hits any limit is reported as failed, never as a success.

### AI chat

Card chat and project chat use the same board tools, but can only read or change cards in the project the chat is open in (for card chat, the project its card is filed in). The one exception is filing: when card chat files an unfiled card, it may create the brand, stream, project or category it files it into. Chat can also set up and run a card's agent — including which tools it's granted — when you ask it to. In **Suggest** mode every change is held for your approval before it's applied.

An external assistant connected through BRUV's MCP server ([docs/mcp-server.md](docs/mcp-server.md)) gets the same board tools across the whole repo, including setting up and running card agents.

## Your API keys

When you add an LLM account, your API key is stored in the **OS keychain** — Windows Credential Manager on Windows, Keychain on macOS, or libsecret on Linux. The `llm_accounts.json` file on disk stores only the non-secret metadata (provider, model, label) with the `api_key` field blank. Values you enter for an MCP server's environment variables are stored the same way.

- Keys never touch the disk in plaintext on systems with a working keychain.
- Keys never leave your machine except in the `Authorization` header of requests to the provider you configured them for.
- You can see BRUV's stored secrets in your OS keychain viewer under the service name **BRUV**.
- If the OS keychain is unavailable (broken libsecret daemon, locked-down corporate machine), BRUV falls back to storing LLM keys in `llm_accounts.json`, as earlier versions did. The goal is to upgrade security when possible, never to lock you out of your own data.
- If you share your config directory (backup, sync tool, etc.), you are **not** sharing your LLM API keys on a system with a working keychain. On a fallback-to-plaintext system, you are sharing the keys; be aware.

If you'd rather not store keys anywhere at all, configure **Ollama** instead and run models locally.

### Other credentials

A few credentials are stored in plain text in the config folder, not in the keychain:

- the SMTP password and webhook `Authorization` header (`notify_config.json`)
- the Trello API key and token (`preferences.json`)
- device tokens for the BRUV servers you connect to (`clientdata/`)

Treat the config folder as private, and don't share it.

### Migrating from earlier versions

If you're upgrading from a BRUV build that predates the keychain backend, the first launch will automatically move any plaintext API keys out of `llm_accounts.json` and into the OS keychain. The migration is one-way and idempotent — it only rewrites the JSON file if there are plaintext keys left to migrate.

## Sharing a repo

BRUV repos are designed to be shared. The repo folder contains everything a project needs — cards, hierarchy, tags, agent configs, card types and templates — and nothing from your config folder. When you share a copy of a repo (zip, git, USB), only the project travels; your AI chats, API keys, and settings stay on your machine.

To work on the *same* repo from several devices, connect them to the machine that hosts it (see [docs/self-hosting.md](docs/self-hosting.md)) rather than syncing the live folder with a file-sync tool such as Syncthing or Dropbox — that isn't a supported way to share a repo.

**What does NOT travel with a shared repo:**

- AI chat history (stored per-user in your config folder, keyed by repo ID)
- LLM API keys (OS keychain)
- Notification history, preferences, profile, window state
- Agent run history (stored per machine in your config folder; configs travel, the record of what your agents actually did stays local)
- The search index and lock file in `.bruv/`

**What DOES travel with a shared repo:**

- Cards, tags, brands, streams, projects, categories
- Card types and templates defined in this repo
- Agent configurations (goals, schedules, tools, budgets, safety rails)
- Attachments and comments
- The activity log, which records the display name of whoever made each change
- Capture and slide-template preferences
- **MCP server definitions** (`mcp_servers.json`) — the command, args, list of env var *names* each server needs, and whether each server is enabled. The recipient sees which servers to install but has to provide their own API keys via the MCP Servers dialog. None of these servers runs on the recipient's machine until they approve it there, after seeing the exact command; approvals stay on each machine, and a changed command needs approving again (see the warning in [README.md](README.md#sharing-a-repo)).

**What does NOT travel with shared MCP configs:**

- API keys or other env var values — these are per-user per-machine via the OS keychain
- Approvals — which servers may run is decided on each machine (`mcp_approvals.json` in the config folder)
- Server-side caches, logs, or auth tokens the subprocess might write to its own config dir

When someone else opens your shared repo, they get a fresh chat history and an empty notification inbox for that repo — their usage stays separate from yours.

### Before sharing

Agent run history (token counts, timestamps, and a truncated record of each tool call and its result) is kept in your config folder under `runs/<repoID>/`, not in the repo, so it doesn't travel. Repos last used with older BRUV versions may still have run history inside their `cards/<id>.agent.json` files; BRUV moves it out the first time it reads each agent. To clear an agent's history yourself, open its runs and choose **Clear**.

What does travel and is worth a glance before you share: comments, the activity log (names of people who made changes), attachments, and agent goals.

## How to wipe everything

Close BRUV, delete `%APPDATA%\bruv\` (and `%PROGRAMDATA%\BRUV\` if you installed the server), restart BRUV. You're back to first-run. LLM and MCP secrets stay in the OS keychain under **BRUV** until you remove them there.

## Questions or concerns

Open an issue on the GitHub repository (linked from the About dialog) or check the source yourself. The outbound calls live in [internal/llm/](internal/llm/) (AI providers), [internal/agent/web.go](internal/agent/web.go) (web tools), [core/capture/](core/capture/) (post capture), [internal/importer/](internal/importer/) (Trello), [internal/notify/](internal/notify/) (email and webhook), [internal/push/](internal/push/) (Web Push) and [internal/update/](internal/update/) (update check); the browser extension's are in [clipper/src/](clipper/src/). There's nothing hidden.
