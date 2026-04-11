<p align="center">
  <img src="frontend/src/assets/images/bruv-icon.svg" alt="BRUV logo" width="128" />
</p>

<h1 align="center">BRUV</h1>

<p align="center"><em>Your most organised best bud.</em></p>

<p align="center">
  An AI-native, local-first productivity app that pairs a kanban-style board with an LLM assistant and autonomous agents — all running on your machine, with your data, under your control.
</p>

> **Status:** public beta (v1.0b). Windows only for now. No telemetry, no account, no cloud. Built with ❤ by Harvey and 🤖 Claude.

---

## What BRUV does

- **Organise work as cards on boards.** Brands → streams → projects → categories → cards. Drag to reorder, pin to multiple places, keep everything sortable and searchable.
- **16 built-in block types.** Text, checklists, selects, numbers, dates, ratings, checkboxes, radios, groups, images, progress bars, alarms, and more. Build your own card schemas without writing a line of code.
- **AI chat on every card, every project.** Three modes: **chat** (ask questions), **suggest** (review AI-proposed edits before they land), **edit** (let the AI mutate cards directly, scoped to the current project).
- **Autonomous agents attached to cards.** Any card can become an agent — schedule it, give it tools (web search, URL fetching, HTTP, notifications, card reads/writes), set a token budget, and let it run. Full run history, cost tracking, safety rails (rate limits, retries, budget caps).
- **Multi-provider LLM support.** Bring your own Anthropic, OpenAI, or Ollama key. Fully local if you use Ollama.
- **Local-first, file-based storage.** All your data is plain JSON in your OS config directory. No database server, no cloud, no account. Back it up with a file copy.
- **System-tray resident.** Minimise to tray, pause all agents from the tray menu, click a notification to jump to the relevant card.

## Download

**Windows:** Grab the latest installer from the [Releases page](https://github.com/harvey-withington/bruv/releases).

> ⚠️ **SmartScreen warning during the beta.** BRUV is not yet code-signed, so Windows SmartScreen will warn you when you run the installer for the first time. This is expected. Click **More info → Run anyway**. The warning will go away in the v1.0 final release once we're code-signed via [SignPath Foundation](https://signpath.org/) (their free OSS sponsorship — no ongoing cost to BRUV, so it stays free for you). See [SmartScreen and signing](#smartscreen-and-signing) below for why.

**macOS / Linux:** not supported yet. See [Platform status](#platform-status).

## Quick start

1. **Install and launch.** BRUV opens to an empty workspace on first run.
2. **Add an LLM provider.** Open **Settings → LLM Accounts** and paste an API key for Anthropic, OpenAI, or point at a local Ollama instance. BRUV works without one — you just won't get AI features until you add one. You'll get a friendly first-run nudge if you skip it.
3. **Create a brand → stream → project.** These are the organisational hierarchy. Think of them as company → department → workstream, or any other three-level grouping that fits your life.
4. **Add categories (columns) to your project, then drop in cards.** Drag to reorder. Every card has a type that determines its block schema.
5. **Open the project chat panel** and ask the AI to help you organise, plan, or draft cards. Try suggest mode if you want to review changes before they land.
6. **Turn a card into an agent.** Open any card → Agent tab → enable an LLM account → pick tools → set a schedule → hit run.

Full keyboard shortcut list: press `?` anywhere in the app.

## Privacy

BRUV is local-first by design. Your data lives in plain JSON on your disk. No telemetry, no analytics, no crash reporting, no account, no cloud.

The only outbound network traffic happens when:
- You use AI chat or run an agent — your prompt goes to **the LLM provider you configured**, and only that provider.
- An agent uses a web tool (`web_search`, `web_fetch`, `http_request`) that you've explicitly enabled for it.

Full details, including what files live where, what agents can access, and how to wipe everything: **[PRIVACY.md](PRIVACY.md)**.

## SmartScreen and signing

BRUV is fully open source and free. Windows code-signing certificates cost real money every year, which doesn't fit a free OSS project maintained by one person. Instead, we're applying to the [SignPath Foundation](https://signpath.org/) — a service that provides free code signing to qualifying open-source projects. Once approved, releases will be signed and SmartScreen will stop warning.

Until then: beta builds ship unsigned. If you'd rather not click through a SmartScreen warning, you can:

- **Verify the binary yourself** against the source — everything here is MIT-licensed and buildable from a clean checkout (see [CONTRIBUTING.md](CONTRIBUTING.md)).
- **Build from source** — clone the repo and run `wails build`.
- **Wait for v1.0 final**, which will be signed.

This isn't a workaround — it's the honest cost of running an unfunded OSS project. Thanks for your patience.

## Platform status

| Platform | Status |
|---|---|
| Windows 10 / 11 | ✅ Supported — public beta |
| macOS | ❌ Not yet. Apple's developer program is $99/year, which doesn't fit the no-recurring-costs model of this beta. Deferred until sponsorship or paid-tier funding covers it. |
| Linux | ❌ Not tested. Wails supports it in principle; no one has smoke-tested a BRUV build on Linux yet. If you'd like to help, open an issue. |

## Support the project

BRUV is free and will stay free. If it saves you time and you'd like to chip in, there'll be a "Buy me a coffee" link in the About dialog at v1.0b. No subscriptions, no locked features.

A future optional hosted sync service is on the roadmap — the desktop app is architected for it via an [adapter pattern](CONTRIBUTING.md#backend-adapter-architecture). The app itself will remain free and local-first forever; the hosted service would be a separate, optional paid add-on.

## Contributing and development

Source, build instructions, architecture notes, and how to add a new backend adapter live in **[CONTRIBUTING.md](CONTRIBUTING.md)**. Pull requests, issues, and thoughtful feedback are welcome.

## License

[MIT](LICENSE) © 2026 Harvey. Free to use, modify, and distribute.
