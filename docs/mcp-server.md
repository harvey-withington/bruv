# BRUV as an MCP server

BRUV exposes each repo to external agentic chat apps (Claude Desktop, etc.)
over the [Model Context Protocol](https://modelcontextprotocol.io). An assistant
can then work on your board straight from a chat — capture ideas into new
Brands, Streams, Projects, Categories and Cards, read and update existing cards,
comment, attach files, and set up and run card agents.

> **Terminology.** On this surface a **Repo** is one BRUV board (its own
> Brands/Streams/Projects/Cards) — *not* a git repository. The word
> "Workspace" is reserved for a separate, future feature and is not used here.
>
> Not to be confused with [docs/mcp-servers.md](mcp-servers.md), which is about
> BRUV *consuming* other MCP servers. This page is BRUV *being* one.

## What it is

- **One MCP server per Repo.** The endpoint is `/repos/<repo-id>/mcp`. The repo
  is fixed by the URL you connect to — it is never chosen by the assistant, so a
  card can't land in the wrong board.
- **Transport:** Streamable HTTP (one JSON-RPC message per POST). Tools only —
  no resources/prompts/sampling.
- **Auth:** the same device bearer token as the mobile app. Reached over
  Tailscale (or any path that reaches the backend).
- **Same tools as BRUV's own AI.** The tool set is BRUV's native board tools —
  the ones card chat, project chat and agents use — over the whole repo. Tools
  create, read, update, file and comment; there are no delete tools. Edits can
  still overwrite content (`set_card_description`, `update_card`), drop tags
  (`remove_card_tags`) or unfile a card (`unpin_card`).
- **Agents:** the connector can read, configure (goal, schedule, granted tools,
  model, budgets) and run card agents. An agent it configures runs on this BRUV
  backend with your LLM accounts and whatever tools it is granted — treat the
  device token accordingly.

## Tools

| Tool | Purpose |
|---|---|
| `list_brands` / `list_streams` / `list_projects` / `list_categories` | Browse the hierarchy. |
| `list_card_types` | Available card types (use as `card_type`; an unknown type is refused, never created). |
| `create_card_type` | Add a new card type — only when the user asks for one or nothing existing fits. Refuses a label that already exists. |
| `get_card` | Read one card by id. |
| `search_cards` | Full-text search (check for duplicates before creating). |
| `create_brand` / `create_stream` / `create_project` / `create_category` | Create hierarchy nodes (parents auto-created). |
| `create_card` | Create + populate a card. Pass all of `brand`/`stream`/`project`/`category` to file it (auto-created), or none to leave it in the inbox. Accepts `tags`, `description`, `blocks`. |
| `add_card_blocks` / `set_card_fields` / `add_card_tags` / `remove_card_tags` | Populate an existing card, or drop tags (`all: true` clears them). |
| `set_card_title` / `set_card_description` / `set_card_type` / `set_card_due_date` | Change a card's intrinsic properties. Description is Markdown; due date is `YYYY-MM-DD` (or an ISO 8601 date-time) or `""` to clear. |
| `add_card_attachment` | Attach a file (≤ 3 MB). Pass `text` for UTF-8 files or `content_base64` for binary — exactly one. |
| `get_card_attachment` | Download an attachment by `attachment_id` or `name`. Text files return as a text block (specs land straight in context); binary as an embedded base64 resource; over 4 MB you get metadata plus a 5-minute signed URL instead. |
| `add_card_comment` / `list_card_comments` | Post or read comments — the natural place for an agent to record an outcome without touching the card's content. |
| `pin_card` / `unpin_card` | File a card into a category (parents auto-created) or remove it from one (nothing created; the card is kept). |
| `list_cards` | Cards on a project board grouped by category in board order — compact summaries; `get_card` for content. |
| `recent_cards` | Most recently updated cards — find what the user just created. |
| `update_card` | Change title, due date, tags and block values in one call (blocks matched by key or label; a new key adds a text block). |
| `get_card_agent` | Read a card's agent: full goal, limits, recent runs, and the valid tool/model options. |
| `configure_card_agent` | Set up or change a card's agent — only the fields passed change; `goal` replaces the whole goal. Can grant tools, set the schedule, model and budgets, and enable it. |
| `run_card_agent` | Run a card's agent now, ignoring its schedule (the card must have a goal). Asynchronous — read the result with `get_card_agent`. |

## Connecting Claude Desktop

You add **one connector per repo** you want the assistant to reach.

### 1. Get a device token

Pair the backend as you would a phone: open the `/pair` URL printed by the
server (`bruv.exe --server`) and enrol, or reuse an existing device token. This
is the same token the mobile app uses.

### 2. Find the repo id

`GET /repos` (with `Authorization: Bearer <token>`) lists `{id, name}` for every
repo. Use the `id` in the connector URL.

### 3a. Add a remote connector (if your client takes a static header)

Settings → Connectors → Add custom connector:

```
https://<your-host>.ts.net/repos/<repo-id>/mcp
Authorization: Bearer <device-token>
```

### 3b. Or use the `mcp-remote` bridge (reliable bearer-token path)

In `claude_desktop_config.json` — one entry per repo:

```json
{
  "mcpServers": {
    "bruv-personal": {
      "command": "npx",
      "args": [
        "-y", "mcp-remote",
        "https://<your-host>.ts.net/repos/<personal-repo-id>/mcp",
        "--header", "Authorization: Bearer ${BRUV_TOKEN}"
      ],
      "env": { "BRUV_TOKEN": "<device-token>" }
    }
  }
}
```

A bad/expired token returns `401`; an unknown/disabled repo id returns `404`.

## How it's wired (for maintainers)

- Tools: [core/boardtools/](../core/boardtools/) is the one registry — `tools.go`
  (handler tables + definitions advertised via `tools/list`), `handlers*.go`
  (implementations), `blocks.go` (argument/block conversion), `native.go` (the
  in-process, optionally project-scoped use by card chat, project chat and
  agents). A tool added there reaches every surface at once.
- Transport: [internal/mcpserver/server.go](../internal/mcpserver/server.go) —
  Streamable HTTP + JSON-RPC dispatch only; `tools/list` and `tools/call`
  delegate to `boardtools.Defs` / `boardtools.Call`.
- Protocol types are reused from [internal/mcp/protocol.go](../internal/mcp/protocol.go)
  (the MCP *client* package).
- Mounted in [transport/http/repos.go](../transport/http/repos.go) `repoRouter`
  as the `mcp` sub-route, behind the existing `requireAuth` wrapper. The handler
  resolves the repo from the URL via the `Supervisor`, so `transport/http` stays
  free of `supervisor` imports (avoids an import cycle).
- Built by the callers: [internal/server/server.go](../internal/server/server.go)
  (headless) and [app.go](../app.go) (desktop loopback), both passing
  `Config.MCPHandler = mcpserver.New(sup, version)`.

## Not in v1

Delete and reorder tools (filing is covered by `pin_card`/`unpin_card`; a move is
an unpin plus a pin), an optional single "all-repos" connector for
cross-repo capture, MCP resources/prompts, OAuth, and repo-scoped tokens.
