# MCP Servers

BRUV supports external tool providers via the [Model Context Protocol](https://modelcontextprotocol.io/) — the same open standard Claude Desktop, VS Code, and a growing list of AI clients use to integrate with local and remote capabilities.

This means your BRUV agents aren't limited to the built-in web/card/system tools. You can install an MCP server for filesystem access, GitHub integration, web scraping with Playwright, flight search, database queries, Slack, Linear, Notion, or any of the dozens of other servers that exist in the ecosystem — and all of them become callable by your agents automatically.

## How it works

MCP servers run as **local subprocesses** on your machine. When a repo is open that has servers configured, BRUV spawns each enabled server, completes a handshake, and asks it what tools it exposes. Those tools then appear in the agent tool catalogue alongside BRUV's built-ins, namespaced as `serverName__toolName` so they never collide.

When an agent calls one of those tools, BRUV forwards the call to the server's subprocess over its standard input, receives the result, and passes it back to the LLM. The server's logs go to BRUV's log output prefixed with the server name, so you can see what's happening.

Three things about this architecture are worth understanding:

1. **MCP servers travel with the repo.** The configuration lives in `.bruv/mcp_servers.json` inside your repo, same as brands, cards, and card types. When you share a repo with someone, they see the same MCP server definitions you do.
2. **API keys and secrets stay on your machine.** The config file lists the *names* of environment variables each server needs, but the values live in the OS keychain keyed by your repo ID, the server name, and the variable name. Sharing a repo never leaks credentials.
3. **Each server runs with a minimal environment.** BRUV passes the current user's `PATH`, `HOME`, `APPDATA`, and a few other essentials — plus whatever secrets the server spec declares — but nothing else from your shell. A server you install can't accidentally see your `AWS_SECRET_ACCESS_KEY` just because you have it in your environment.

## Adding a server

1. Open any repo in BRUV
2. Click the **Plug** icon in the top toolbar (or open the MCP Servers dialog however your shortcuts are configured)
3. Click **Add**
4. Fill in:
   - **Name** — a short unique identifier for this repo. Used as a namespace prefix for the server's tools. Immutable after creation.
   - **Command** — the executable to run. Usually `npx` for Node-based servers, or an absolute path.
   - **Arguments** — one argument per field, passed to the command. For `npx` servers this is typically `-y` followed by the npm package name, followed by any server-specific args.
   - **Environment variables** — the names of any env vars the server needs. Values go in the secret fields below.
   - **Enabled** — toggle off to keep the config but not spawn the server.
5. Click **Save**. BRUV will spawn the server, handshake, and discover its tools. The status indicator tells you whether it came up successfully.

Once a server is ready, its tools appear in every agent's allowed-tool picker. Agents don't automatically get access to new tools — you still enable them per-agent, just as you do for the built-in tools.

## Example configurations

### Filesystem (official reference server)

Gives agents read/write access to a specific directory tree. Good for agents that need to read your notes, search through files, or write generated content to disk.

- **Name:** `filesystem`
- **Command:** `npx`
- **Arguments:** `-y`, `@modelcontextprotocol/server-filesystem`, `C:\Users\harve\Documents\notes` *(replace with the directory you want the server to have access to — the server enforces this as its boundary)*
- **Environment variables:** none
- **Tools exposed:** `read_text_file`, `write_file`, `edit_file`, `create_directory`, `list_directory`, `search_files`, `directory_tree`, `get_file_info`, `list_allowed_directories`, and more

You can pass multiple directory arguments to sandbox multiple roots at once: `-y`, `@modelcontextprotocol/server-filesystem`, `C:\work\notes`, `C:\work\project`.

### GitHub

Lets agents read issues, PRs, commits, and repository content. Requires a personal access token.

- **Name:** `github`
- **Command:** `npx`
- **Arguments:** `-y`, `@modelcontextprotocol/server-github`
- **Environment variables:** `GITHUB_PERSONAL_ACCESS_TOKEN`
- **Tools exposed:** `create_issue`, `list_issues`, `get_issue`, `create_pull_request`, `list_pull_requests`, `search_code`, `get_file_contents`, and many more

After saving the server config, click **Edit** on the new server and paste your GitHub token into the `GITHUB_PERSONAL_ACCESS_TOKEN` field. The token is stored in your OS keychain — not in the repo — and is passed to the server subprocess at spawn time.

### Playwright (web scraping with a real browser)

For the hard cases where JS-heavy sites like flight aggregators, hotel search, or stock tickers can't be scraped with plain HTTP. The Playwright MCP server runs a real headless Chromium and gives agents the ability to navigate, click, fill forms, and extract rendered content.

- **Name:** `playwright`
- **Command:** `npx`
- **Arguments:** `-y`, `@playwright/mcp`
- **Environment variables:** none (Playwright downloads its own browser binaries on first run)
- **Tools exposed:** `browser_navigate`, `browser_click`, `browser_type`, `browser_snapshot`, `browser_wait_for`, and more

**Note:** Playwright downloads a Chromium binary (~200 MB) on first use. This happens inside the server subprocess — BRUV doesn't manage it. Be patient the first time.

## Security posture

MCP servers are **third-party code running on your machine**. BRUV treats them the same way any desktop app treats plugins: the user is responsible for choosing which ones to install, and BRUV provides the smallest reasonable sandbox (minimal env, stdio-only transport, no network exposure) but does not promise isolation from anything a subprocess running as your user could already do.

Things to keep in mind:

- **Install only servers you trust.** Prefer official servers from `@modelcontextprotocol/` or well-known authors. Check the source before running unfamiliar packages.
- **Scope filesystem servers narrowly.** If you use the filesystem server, pass it the specific directory you want it to access — not your home directory or C:\.
- **Audit env var names.** A malicious server spec could declare an env var name that, combined with a credential you happen to paste, gives it access to something you didn't intend. Review the env var names in any shared repo before pasting secrets.
- **Agent permissions are per-card.** Even after installing an MCP server, you still enable specific tools per-agent via the Tool Permissions UI. A server being installed doesn't mean every agent can use it.

## Troubleshooting

**Server shows "failed" in the list**
- Click **Edit** and verify the command and args are correct
- Click the refresh icon to retry the spawn
- Check the BRUV log for the server's stderr output — most MCP servers print useful startup errors there
- For `npx` servers: make sure Node.js is installed and `npx` is on your PATH

**Server is "ready" but the agent says tool calls are failing**
- Check that the server has the env vars it needs — the Edit dialog shows "set" or "not set" next to each declared name
- Try calling the tool from a test agent first; error messages from the server come back verbatim
- If the server worked before and suddenly doesn't, click **Restart** — sometimes servers get into a weird state after a long idle period

**Sharing a repo: the recipient sees no tools from my servers**
- They need to install each server's dependencies themselves (for `npx` servers this happens automatically on first run)
- They need to paste their own secret values for any env vars the server declared — your keys don't travel

**Tools don't appear in the agent permissions picker**
- The server has to be in "ready" state before its tools are catalogued
- Restart the server from the MCP Servers dialog; if that doesn't help, close and re-open the repo

## Further reading

- [Model Context Protocol spec](https://modelcontextprotocol.io/specification/2025-06-18) — the full protocol reference
- [Official MCP server list](https://github.com/modelcontextprotocol/servers) — Anthropic's reference implementations for common integrations
- [Community MCP servers](https://github.com/punkpeye/awesome-mcp-servers) — a broader catalogue of third-party servers
