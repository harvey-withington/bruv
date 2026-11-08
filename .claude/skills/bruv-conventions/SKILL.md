---
name: bruv-conventions
description: BRUV feature/commit checklist — walk it when building features or preparing a commit. Covers both-surface parity, localization, error surfacing, shared/ placement, and the verification commands CI will hold you to.
---

# BRUV Conventions Checklist

CLAUDE.md holds the rules; this skill is the *procedure* — what to actually check and run. Walk it when implementing a feature, and again before committing.

## Architecture map (30 seconds)

- **Go backend at repo root** (`core/` services + runtime, `transport/http` JSON-RPC 2.0 + SSE, `internal/` repo/model/index). Wails desktop shell and headless `bruv.exe --server` drive the same service layer.
- **`frontend/`** — desktop Svelte app (Wails). **`mobile/`** — phone PWA served at `/m/`. Two surfaces of one app.
- **`shared/`** — TypeScript used by both surfaces (`@shared/` alias in both vite configs). API wrappers (`api.ts`), backend interface types (`types.ts`), and transport-agnostic logic.
- **`plan/`** — gitignored here; for the maintainer it's a separate private repo cloned into this folder, so it travels between their machines. `plan/TODO.md` is the single source of truth for open work; check it before proposing new work, update it when scope changes. `plan/claude-notes.md` holds the maintainer's private working notes (imported by CLAUDE.md when present).

## Design ethos (the why behind the rules)

BRUV is a polished Kanban surface that **augments an AI-first capture ethos** — the app brings the structure (Brand → Stream → Project → Category → Card, Card Types, a shared icon vocabulary) so capture is just "drop it in the right shape." Distinct from Trello by being more private/repo-based, MCP-native, and deliberately **more opinionated**. Three north stars that should shape feature decisions:

- **Opinionated defaults over customization knobs.** A strong, consistent default *is* the feature. Don't add theming/configurability just because you can — resist letting users redesign the app. ("Facebook is blue. Deal with it.")
- **Icons are the primary way to distinguish elements** — reach for an icon before colour-blocking or imagery.
- **Uniform card size is sacred.** No feature may make cards variable-height. Card Covers are intentionally omitted; if ever added they must be *subtle* and **must not change card dimensions**. Treat any proposal that grows/varies card height as wrong by default.

## Feature checklist

1. **Both surfaces?** Default is parity between `frontend/` and `mobile/`. Intentional asymmetries exist — mobile is the capture/triage/review/chat surface; structure authoring, agent authoring, and LLM/MCP config are desktop-only by design (see `plan/mobile-feature-gap-2026-05-03.md` for the bucket list). If you implement one side only, confirm it's a deliberate asymmetry and say so.
2. **Localization.** Every user-facing string — labels, placeholders, tooltips, `aria-label`s, errors, confirmations — goes through `t()` from `lib/i18n.svelte`. Add keys to **both** `frontend/src/lib/locales/en.json` and `mobile/src/lib/locales/en.json` when the feature spans surfaces. Params use `{name}` interpolation. Strings inside shared modules must be injected by callers (see `cardToMarkdown`'s `labels` option / `cardMarkdownLabels()` for the pattern) — shared code never imports a surface's i18n.
3. **Error surfacing.** `catch { console.error }` is banned when the user is affected. Both surfaces: `showToast(t('...'), 'error')` from `lib/toast.svelte` (shared store in `@shared/toast.svelte`; each surface has its own Toast component). Toasts survive navigation. Mobile pages also have inline `saveError`/`mutationError` rails — prefer those for persistent field-level errors, toasts for transient op feedback.
   Success feedback rule: **one-shot actions toast** (copy, export, import, delete); **ambient autosave state stays inline** (desktop `SaveIndicator`, mobile saved-chip — UI-CONVENTIONS.md §9). Never toast per-keystroke/debounced save events.
4. **Confirmations.** Never native `confirm()`/`alert()`. Desktop: `await showConfirm(...)` from `lib/confirm.svelte`. Mobile: prop-based `ConfirmDialog` component.
5. **Layered dialogs shield BOTH window keys.** A dialog/sheet/picker layered above a closable container (CardDetail, mobile sheets) must intercept **Escape AND Ctrl+Enter** — the parent's `<svelte:window>` handler owns both, and an unshielded chord closes the card underneath, discarding the layer's edits. This bug has recurred on four dialogs; Escape-only shields are half a fix. Patterns (capture-phase shield vs parent stand-down guard) + references: UI-CONVENTIONS §8.1.
6. **Shared logic placement.** Logic needed by both surfaces goes in `shared/`, transport-agnostic, with the backend surface injected as a small interface — desktop binds `@shared/api` wrappers, mobile binds `repoRPC`. Reference implementation: `shared/cardTransfer.ts` with the two ~50-line `cardExport.ts` adapters.
7. **Reusable behaviours.** Check `frontend/src/lib/actions.ts` (focusTrap, focusOnMount, floatingDropdown…) and `mobile/src/lib/actions/` before writing new DOM logic. Extract on second use.
8. **Design tokens.** Colors/sizing via CSS custom properties (`var(--text-muted)` etc.). No inline hex/rgb in components.
9. **State keyed by entity ID**, never array index — indices shift on reorder/delete.
10. **Component size ~300 lines.** If your change pushes a component further past it, extract the piece you're adding as a child component instead of growing the parent.
11. **Strict TS.** No `any`, no `as any`. If a backend method in `shared/types.ts` returns `Promise<any>`, prefer fixing its type over casting at the call site.
12. **Drag-and-drop** over up/down buttons wherever reordering exists. Design metaphor: no grip-handle icons — the element body drags. Mobile uses the Pointer-Events action in `mobile/src/lib/actions/dnd.svelte.ts` (long-press to arm); desktop uses HTML5 drag via `frontend/src/lib/reorder.ts`. The split is intentional (touch needs long-press) — don't unify them.
13. **UI-CONVENTIONS is a contract** — it lives at `frontend/src/UI-CONVENTIONS.md` (NOT the repo root — root-level searches miss it). Update it when adding shared components or patterns.
14. **Fix every sibling copy in the same change.** When a fix or a new shared helper applies elsewhere (duplicate helpers, sibling components, the other surface), grep for the pattern and convert every hit now, not as a follow-up. Ask only when a sibling genuinely means something different.
15. **One tool registry.** Every LLM surface (MCP, card chat, project chat, agents) uses `core/boardtools` — handlers, schemas and scope together. Change the registry or the shared helper, never a surface copy, schemas included. `TestNoSurfaceRedefinesANativeTool` guards the names. Agents: an empty `allowed_tools` grants no tools.
16. **Card types must exist.** Never stamp a card with a type id that isn't defined (there is no built-in "idea" type; defaults use `catalog.DefaultCardType`). Every LLM surface refuses an unknown type and names the known ones; creating a type is an explicit tool call, used sparingly. Use real type ids in examples, tool descriptions and tests.
17. **Setup fixes are durable.** The project is built on more than one machine, and "works on the other machine" is usually environment state (tool versions on PATH, an `npm install` never run). Fix it where every machine gets it (postinstall, version pins in CONTRIBUTING.md, scripts), and compare `wails version` / `node -v` / `go version` before suspecting the code.

## Public repo hygiene

The repo is public. Tracked files (code, comments, tests, docs, skills) must read as written for an unknown developer: "your home server", `<host>`, env vars such as `BRUV_DEPLOY_HOST`. Never a real hostname, tailnet, SSH user, machine name, or anything about the maintainer's habits. Test fixtures use neutral names (`homebox.example-tailnet.ts.net`, `HOMEBOX`). Keep the lesson, drop the diary. Private specifics go in `plan/`. Before committing a new skill, doc or comment, grep it for the identifiers listed in `plan/claude-notes.md`.

## Working on Windows

- Commands default to PowerShell (the VS Code terminal here). Say so when a snippet is bash-only (`$VAR`, `rm -rf`, heredocs); plain `git` / `npm` / `go` need no note.
- In the Bash tool (Git Bash), a heredoc whose body contains backticks fails with "unexpected EOF". Write such files or scripts with the Write tool instead.
- Windows PowerShell 5.1 turns a native command's stderr (git's push progress, npm warnings) into an error and a non-zero `$?`, even when the command succeeded. Judge by the command's own output and exit code.

## Pre-commit verification

Run from the repo root. These are the same commands CI runs:

```powershell
npm run check   # go vet + svelte-check --fail-on-warnings on both surfaces (ANY warning fails)
npm test        # go test (root + foldertemplate module) + vitest on both surfaces
```

CI also fails on unformatted Go: `gofmt -l $(git ls-files '*.go')` must print nothing. `.gitattributes` keeps the working tree LF, so a non-empty list is a real formatting change, not CRLF noise.

**check + test is not everything CI does.** Before any push that touched shared/, adapters, the Go App/ShellAPI surface, or vite configs, also mirror the build steps. These have caught real breaks that check and test missed:

```powershell
wails generate module                        # CI regenerates bindings from the Go struct; stale local wailsjs can mask mismatches
cd frontend; npm run build                   # production bundles are a separate gate from svelte-check
cd ../mobile; npm run build
```

Known CI quirk: the internal/mcp integration tests spawn the real MCP filesystem server via npx; ci.yml pre-warms the npx cache before `go test`. If they flake on a runner, suspect cold-cache install time, not the code.

Grep guards (should return nothing, or only justified hits):

```powershell
# swallowed errors in user-affecting flows
rg -n "catch\s*\(?\s*\w*\s*\)?\s*\{\s*console\.(error|warn|log)" frontend/src mobile/src
# native dialogs
rg -n "\b(window\.)?(confirm|alert)\(" frontend/src mobile/src --glob "*.svelte"
# strict-TS violations
rg -n "as any|: any\b|Promise<any>" frontend/src mobile/src shared
```

Fix root causes, not symptoms — no `svelte-ignore`, no `--no-verify`.

## Commits

**Don't run `git commit` or `git push` unless the developer asks for it in the moment** — they normally commit and push manually after verifying the work. An ask covers that one commit; push only when push is asked for too, and neither carries over to later work. When committing before a deploy, commit first so the build stamps the right sha. Otherwise finish by suggesting a commit message: compact one-liner, lowercase, items separated by " / " (slashes, not commas):

```
card share/export as markdown + json / import from json / fix pin-rejection orphan
```

Present the message — and any other text the developer is meant to copy (commands, config values) — in its own fenced code block, never inline code, so it gets a copy button. Keep it plain ASCII.
