# BRUV as an MCP server

BRUV exposes each repo to external agentic chat apps (Claude Desktop, etc.)
over the [Model Context Protocol](https://modelcontextprotocol.io). An assistant
can then help you **capture ideas and inspiration** — creating Brands, Streams,
Projects, Categories and Cards, and populating cards — straight from a chat.

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
- **Capture-focused:** create + populate + read/search. No move/delete/destroy
  in v1.

## Tools

| Tool | Purpose |
|---|---|
| `list_brands` / `list_streams` / `list_projects` / `list_categories` | Browse the hierarchy. |
| `list_card_types` | Available card types (use as `card_type`). |
| `get_card` | Read one card by id. |
| `search_cards` | Full-text search (check for duplicates before creating). |
| `create_brand` / `create_stream` / `create_project` / `create_category` | Create hierarchy nodes (parents auto-created). |
| `create_card` | Create + populate a card. Pass all of `brand`/`stream`/`project`/`category` to file it (auto-created), or none to leave it in the inbox. Accepts `tags`, `description`, `blocks`. |
| `add_card_blocks` / `set_card_fields` / `add_card_tags` | Populate an existing card. |

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

- Package: [internal/mcpserver/](../internal/mcpserver/) — `server.go` (transport
  + JSON-RPC dispatch), `tools.go` (tool registry + definitions), `handlers.go`
  (tool implementations), `blocks.go` (arg/block conversion).
- Protocol types are reused from [internal/mcp/protocol.go](../internal/mcp/protocol.go)
  (the MCP *client* package).
- Mounted in [transport/http/repos.go](../transport/http/repos.go) `repoRouter`
  as the `mcp` sub-route, behind the existing `requireAuth` wrapper. The handler
  resolves the repo from the URL via the `Supervisor`, so `transport/http` stays
  free of `supervisor` imports (avoids an import cycle).
- Built by the callers: [internal/server/server.go](../internal/server/server.go)
  (headless) and [app.go](../app.go) (desktop loopback), both passing
  `Config.MCPHandler = mcpserver.New(sup, version)`.
- Tool writes go through the same `Runtime` methods and block coercion
  (`CoerceBlockValueForBlock`) as the internal AI chat, so behaviour matches.

## Not in v1 (see the plan)

Move/delete/reorder tools, an optional single "all-repos" connector for
cross-repo capture, MCP resources/prompts, OAuth, and repo-scoped tokens. See
[plan/bruv-mcp-server-for-third-party-agents-2026-06-19.md](../plan/bruv-mcp-server-for-third-party-agents-2026-06-19.md).
