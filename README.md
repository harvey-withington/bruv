<p align="center">
  <img src="frontend/src/assets/images/bruv-icon.svg" alt="BRUV logo" width="128" />
</p>

<h1 align="center">BRUV</h1>

<p align="center"><em>Your most organised best bud.</em></p>

<p align="center">
  An AI-native, local-first productivity app that pairs a kanban-style board with an LLM assistant and autonomous agents — all running on your machine, with your data, under your control.
</p>

> **Status:** public alpha (v1.0a). Windows only for now. No telemetry, no account, no cloud. Built with ❤ by Harvey and 🤖 Claude.

---

## What BRUV does

- **Organise work as cards on boards.** Brands → streams → projects → categories → cards. Drag to reorder, pin to multiple places, keep everything sortable and searchable.
- **16 built-in block types.** Text, checklists, selects, numbers, dates, ratings, checkboxes, radios, groups, images, progress bars, alarms, and more. Build your own card schemas without writing a line of code.
- **AI chat on every card, every project.** Three modes: **chat** (ask questions), **suggest** (review AI-proposed edits before they land), **edit** (let the AI mutate cards directly, scoped to the current project).
- **Autonomous agents attached to cards.** Any card can become an agent — schedule it, give it tools (web search, URL fetching, HTTP, notifications, card reads/writes), set a token budget, and let it run. Full run history, cost tracking, safety rails (rate limits, retries, budget caps).
- **Pluggable external tools via MCP.** Beyond the built-in tools, agents can use any [Model Context Protocol](https://modelcontextprotocol.io/) server you install — filesystem access, GitHub integration, Playwright scraping, flight/hotel APIs, database queries, the full ecosystem. Configuration is per-repo and travels with the project when shared; API keys stay in your OS keychain and never leak. See [docs/mcp-servers.md](docs/mcp-servers.md).
- **Multi-provider LLM support.** Bring your own Anthropic, OpenAI, or Ollama key. Fully local if you use Ollama.
- **Local-first, file-based storage.** All your data is plain JSON in your OS config directory. No database server, no cloud, no account. Back it up with a file copy.
- **System-tray resident.** Minimise to tray, pause all agents from the tray menu, click a notification to jump to the relevant card.

## Download

**Windows:** Grab the latest installer from the [Releases page](https://github.com/harvey-withington/bruv/releases).

> ⚠️ **SmartScreen warning during the alpha.** BRUV is not yet code-signed, so Windows SmartScreen will warn you when you run the installer for the first time. This is expected. Click **More info → Run anyway**. The warning will go away in the v1.0 final release once we're code-signed via [SignPath Foundation](https://signpath.org/) (their free OSS sponsorship — no ongoing cost to BRUV, so it stays free for you). See [SmartScreen and signing](#smartscreen-and-signing) below for why.

**macOS / Linux:** not supported yet. See [Platform status](#platform-status).

## Quick start

1. **Install and launch.** BRUV opens to an empty workspace on first run.
2. **Add an LLM provider.** Open **Settings → LLM Accounts** and paste an API key for Anthropic, OpenAI, or point at a local Ollama instance. BRUV works without one — you just won't get AI features until you add one. You'll get a friendly first-run nudge if you skip it.
3. **Create a brand → stream → project.** These are the organisational hierarchy. Think of them as company → department → workstream, or any other three-level grouping that fits your life.
4. **Add categories (columns) to your project, then drop in cards.** Drag to reorder. Every card has a type that determines its block schema.
5. **Open the project chat panel** and ask the AI to help you organise, plan, or draft cards. Try suggest mode if you want to review changes before they land.
6. **Turn a card into an agent.** Open any card → Agent tab → enable an LLM account → pick tools → set a schedule → hit run.

Full keyboard shortcut list: press `?` anywhere in the app.

## Sharing a repo

BRUV repos are self-contained and portable. To share a project with someone else — or sync your work across machines — zip the repo folder, commit it to git, or drop it in any sync service. Everything the project needs is inside:

- Cards, tags, and the full brand → stream → project → category hierarchy
- Agent configurations (schedules, tools, budgets)
- Your custom card types and templates for that repo
- Attachments and comments

Your personal data stays on your machine and does **not** travel with a shared repo: AI chat history, LLM API keys, notification history, profile, and window state all live in your local config folder keyed per-repo. When a collaborator opens your shared repo, they get their own fresh chat history — your conversations stay private.

BRUV's **Import card types from another repo** button (Card Types dialog) lets you pull a type vocabulary from another local repo without an intermediate export file — useful when you maintain several repos and want to keep a shared set of types across them.

## Self-hosting (one server, multiple devices)

BRUV can run as a Windows Service on a home machine, with other devices on your tailnet pointing at it through the desktop app's **Connections** dialog. One repo, many devices — laptop, partner's PC, a phone in the browser, all editing the same data. Tick the **Server** box on the installer's components page; the rest is one click.

Full walkthrough (Tailscale setup, day-two operations, troubleshooting): **[docs/self-hosting.md](docs/self-hosting.md)**.

## Privacy

BRUV is local-first by design. Your data lives in plain JSON on your disk. No telemetry, no analytics, no crash reporting, no account, no cloud.

The only outbound network traffic happens when:
- You use AI chat or run an agent — your prompt goes to **the LLM provider you configured**, and only that provider.
- An agent uses a web tool (`web_search`, `web_fetch`, `http_request`) that you've explicitly enabled for it.

Full details, including what files live where, what agents can access, and how to wipe everything: **[PRIVACY.md](PRIVACY.md)**.

## SmartScreen and signing

BRUV is fully open source and free. Windows code-signing certificates cost real money every year, which doesn't fit a free OSS project maintained by one person. Instead, we're applying to the [SignPath Foundation](https://signpath.org/) — a service that provides free code signing to qualifying open-source projects. Once approved, releases will be signed and SmartScreen will stop warning.

Until then: alpha builds ship unsigned. If you'd rather not click through a SmartScreen warning, you can:

- **Verify the binary yourself** against the source — everything here is MIT-licensed and buildable from a clean checkout (see [CONTRIBUTING.md](CONTRIBUTING.md)).
- **Build from source** — clone the repo and run `wails build`.
- **Wait for v1.0 final**, which will be signed.

This isn't a workaround — it's the honest cost of running an unfunded OSS project. Thanks for your patience.

The alpha releases use the `v1.0a` tag family; betas will become `v1.0b` once we're signed and ready for a wider audience.

## Platform status

| Platform | Build | Installer | Tray | Notes |
|---|---|---|---|---|
| Windows 10 / 11 | ✅ | ✅ NSIS | ✅ | Primary release target. Public alpha. |
| Linux | ✅ | ⚠️ none | ❌ | Backend cross-compiles cleanly; CI smoke-tests every push. No official installer or tray support yet — build from source if you want to try it. Contributions welcome. |
| macOS | ✅ | ❌ | ❌ | Backend cross-compiles and CI smoke-tests every push. No code-signed release because Apple's developer program is $99/year, which doesn't fit the no-recurring-costs model of this alpha. Deferred until sponsorship or paid-tier funding covers it. |

The tray icon is a Windows-only feature for now. On macOS and Linux, BRUV runs as a normal windowed app — the agent scheduler and all other features work identically.

## Support the project

BRUV is free and will stay free. If it saves you time and you'd like to chip in, there'll be a "Buy me a coffee" link in the About dialog at v1.0 final. No subscriptions, no locked features.

A future optional hosted sync service is on the roadmap — the desktop app is architected for it via an [adapter pattern](CONTRIBUTING.md#backend-adapter-architecture). The app itself will remain free and local-first forever; the hosted service would be a separate, optional paid add-on.

Hey, and coffee keeps the ideas coming!

[![Ko-fi](https://img.shields.io/badge/Ko--fi-Support%20my%20work-ff5e5b?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/harveywithington)

## Contributing and development

Source, build instructions, architecture notes, and how to add a new backend adapter live in **[CONTRIBUTING.md](CONTRIBUTING.md)**. Pull requests, issues, and thoughtful feedback are welcome.

## License

[MIT](LICENSE) © 2026 Harvey. Free to use, modify, and distribute.
