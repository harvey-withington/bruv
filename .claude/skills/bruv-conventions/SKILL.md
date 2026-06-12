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
- **`plan/`** — gitignored project journal. `plan/TODO.md` is the single source of truth for open work; check it before proposing new work, update it when scope changes.

## Feature checklist

1. **Both surfaces?** Default is parity between `frontend/` and `mobile/`. Intentional asymmetries exist — mobile is the capture/triage/review/chat surface; structure authoring, agent authoring, and LLM/MCP config are desktop-only by design (see `plan/mobile-feature-gap-2026-05-03.md` for the bucket list). If you implement one side only, confirm it's a deliberate asymmetry and say so.
2. **Localization.** Every user-facing string — labels, placeholders, tooltips, `aria-label`s, errors, confirmations — goes through `t()` from `lib/i18n.svelte`. Add keys to **both** `frontend/src/lib/locales/en.json` and `mobile/src/lib/locales/en.json` when the feature spans surfaces. Params use `{name}` interpolation. Strings inside shared modules must be injected by callers (see `cardToMarkdown`'s `labels` option / `cardMarkdownLabels()` for the pattern) — shared code never imports a surface's i18n.
3. **Error surfacing.** `catch { console.error }` is banned when the user is affected. Both surfaces: `showToast(t('...'), 'error')` from `lib/toast.svelte` (shared store in `@shared/toast.svelte`; each surface has its own Toast component). Toasts survive navigation. Mobile pages also have inline `saveError`/`mutationError` rails — prefer those for persistent field-level errors, toasts for transient op feedback.
   Success feedback rule: **one-shot actions toast** (copy, export, import, delete); **ambient autosave state stays inline** (desktop `SaveIndicator`, mobile saved-chip — UI-CONVENTIONS.md §9). Never toast per-keystroke/debounced save events.
4. **Confirmations.** Never native `confirm()`/`alert()`. Desktop: `await showConfirm(...)` from `lib/confirm.svelte`. Mobile: prop-based `ConfirmDialog` component.
5. **Shared logic placement.** Logic needed by both surfaces goes in `shared/`, transport-agnostic, with the backend surface injected as a small interface — desktop binds `@shared/api` wrappers, mobile binds `repoRPC`. Reference implementation: `shared/cardTransfer.ts` with the two ~50-line `cardExport.ts` adapters.
6. **Reusable behaviours.** Check `frontend/src/lib/actions.ts` (focusTrap, focusOnMount, floatingDropdown…) and `mobile/src/lib/actions/` before writing new DOM logic. Extract on second use.
7. **Design tokens.** Colors/sizing via CSS custom properties (`var(--text-muted)` etc.). No inline hex/rgb in components.
8. **State keyed by entity ID**, never array index — indices shift on reorder/delete.
9. **Component size ~300 lines.** If your change pushes a component further past it, extract the piece you're adding as a child component instead of growing the parent.
10. **Strict TS.** No `any`, no `as any`. If a backend method in `shared/types.ts` returns `Promise<any>`, prefer fixing its type over casting at the call site.
11. **Drag-and-drop** over up/down buttons wherever reordering exists. Design metaphor: no grip-handle icons — the element body drags. Mobile uses the Pointer-Events action in `mobile/src/lib/actions/dnd.svelte.ts` (long-press to arm).
12. **UI-CONVENTIONS.md is a contract** — update it when adding shared components or patterns.

## Pre-commit verification

Run from the repo root (PowerShell):

```powershell
cd frontend; npm run check; npx vitest run   # desktop: types + tests
cd ../mobile; npm run check                  # CI runs this with --fail-on-warnings: ANY warning breaks the build
```

If Go code was touched: `go test ./...` and `go vet ./...`.

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

**Never run `git commit` or `git push` — Harvey always commits and pushes manually.** Finish by suggesting a commit message: compact one-liner, lowercase, items separated by " / " (slashes, not commas):

```
card share/export as markdown + json / import from json / fix pin-rejection orphan
```
