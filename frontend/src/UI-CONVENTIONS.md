# UI Conventions

This document describes the shared UI patterns and reusable components used across the BRUV frontend. Follow these conventions when building new UI to ensure consistent, accessible behaviour.

---

## 1. Reveal-on-Hover/Focus Action Buttons

**Purpose:** Buttons that are invisible until the user hovers over their parent row or focuses them via keyboard.

**CSS classes** (defined in `style.css`):

| Class | Role |
|---|---|
| `.action-reveal-parent` | Add to the **parent** row/container. On hover, all child `.action-reveal` elements become visible. |
| `.action-reveal` | Add to each **button**. Handles transparent→visible transition on parent hover and on `:focus-visible`. |
| `.action-reveal--edit` | Variant — accent colour on hover/focus. |
| `.action-reveal--danger` | Variant — red/danger colour on hover/focus. |

**Example:**

```svelte
<div class="my-row action-reveal-parent">
  <span class="label">Item name</span>
  <button class="action-reveal action-reveal--edit" title="Rename"><Pencil size={12} /></button>
  <button class="action-reveal action-reveal--danger" title="Delete"><Trash2 size={12} /></button>
</div>
```

**Key behaviours:**
- Buttons are `color: transparent` by default (invisible but still in the DOM for screen readers).
- On parent hover → `color: var(--text-faint)`.
- On button hover → variant colour (accent or danger).
- On `:focus-visible` → same colour as hover, so keyboard users see the same state.

**Where used:** Sidebar tree rows, CardDetail block headers, WelcomeScreen recent items, EditableChecklist items.

---

## 2. EditableText Component

**File:** `components/EditableText.svelte`

A click-to-edit text field with full keyboard accessibility.

**Props:**

| Prop | Type | Default | Description |
|---|---|---|---|
| `value` | `string` | `''` | Current text value |
| `placeholder` | `string` | `'Click to edit'` | Shown when value is empty |
| `multiline` | `boolean` | `false` | Use `<textarea>` instead of `<input>` |
| `markdown` | `boolean` | `false` | Render value as full block markdown when not editing |
| `inlineMarkdown` | `boolean` | `false` | Render value as inline markdown (no block elements) when not editing |
| `rows` | `number` | `3` | Textarea row count (only used when `multiline` is true) |
| `class` | `string` | `''` | Extra CSS class applied to the root element |
| `onSave` | `(value: string) => void` | — | Called when the user commits a change |
| `onCancel` | `() => void` | — | Called when the user cancels (Escape) |
| `onTab` | `() => void` | — | Called on Tab keypress — useful for moving focus to the next field |

**Keyboard behaviour:**
- **Click** or **Enter** (when focused on the display) → enters edit mode, selects all text.
- **Enter** → saves (single-line mode); **Ctrl+Enter** → saves (multiline mode).
- **Escape** → cancels edit, reverts to original value, calls `onCancel`.
- **Tab** → calls `onTab` if provided; otherwise default tab behaviour.
- **Blur** → saves.

**Example:**

```svelte
<EditableText
  value={card.title}
  placeholder="Untitled"
  onSave={(v) => updateTitle(v)}
  onTab={() => focusDescription()}
/>
```

---

## 3. EditableChecklist Component

**File:** `components/EditableChecklist.svelte`

A checklist with inline editing, toggle, add, and remove — all keyboard accessible.

**Props:**

| Prop | Type | Description |
|---|---|---|
| `items` | `Array<{ id: string, text: string, done: boolean }>` | Current checklist items |
| `onUpdate` | `(items: Array<...>) => void` | Called whenever items change (toggle, edit, add, remove) |

**Keyboard behaviour per row:**
- **Tab** order: checkbox → text (click-to-edit) → delete button.
- **Click** on text → enters inline edit mode.
- **Enter** in edit → saves text.
- **Escape** in edit → cancels.
- **Tab** from edit → saves and moves focus to the delete button on the same row.

**Add-item input** at the bottom accepts Enter to add.

**Example:**

```svelte
<EditableChecklist
  items={checklistItems}
  onUpdate={(updated) => saveChecklist(updated)}
/>
```

---

## 4. Inline Edit Input Styling

**CSS classes** (defined in `style.css`):

| Class | Role |
|---|---|
| `.inline-edit-input` | Consistent styling for inline text inputs (background, border, border-radius, focus ring). |
| `.editable-display` | Styling for the non-editing display state (subtle hover background to hint editability). |

Use these when building custom editable fields that don't use the `EditableText` component.

---

## 5. ConfirmDialog — Destructive Action Confirmation

**Files:** `components/ConfirmDialog.svelte`, `lib/confirm.svelte.ts`

All destructive actions (delete, unpin, etc.) must use the in-app confirm dialog. **Never** use `window.confirm()`.

**Usage:**

```typescript
import { showConfirm } from '../lib/confirm.svelte'

async function handleDelete() {
  if (!await showConfirm('Delete this card? This cannot be undone.')) return
  await DeleteCard(id)
}
```

`showConfirm(message)` returns a `Promise<boolean>` — `true` if the user confirmed, `false` if cancelled.

**Mounting:** `<ConfirmDialog />` is mounted once in `App.svelte` outside all conditional blocks. Do not mount it elsewhere.

**Keyboard:** Enter confirms, Escape cancels, click-outside cancels.

---

## 6. Toast Notifications — User-Visible Errors & Feedback

**Files:** `components/Toast.svelte`, `lib/toast.svelte.ts`

All errors from API calls and all user-facing feedback must use toasts. **Never** use `window.alert()` or silent `console.error`.

**Usage:**

```typescript
import { showToast } from '../lib/toast.svelte'

try {
  await SaveSomething()
  showToast(t('common.saved'), 'success')
} catch (e) {
  showToast(t('error.save_failed'), 'error')
}
```

**Toast types:** `'info'` | `'success'` | `'error'` | `'warning'`

**Duration:** 4 seconds by default. Pass a third argument (ms) to override.

**Mounting:** `<Toast />` is mounted once in `App.svelte`. Do not mount it elsewhere.

---

## 7. `focusOnMount` — Svelte Action for Auto-Focus

**File:** `lib/actions.ts`

Focuses an input or textarea when it mounts. Use this instead of `$effect` focus blocks. Pair with conditional rendering — the element should only mount when it should receive focus.

```svelte
{#if editing}
  <input use:focusOnMount={true} bind:value={draft} />
{/if}
```

Pass `true` (or any truthy value) to also select all text on mount (ideal for rename inputs).  
Omit the argument (or pass `false`) for focus-only (ideal for textareas).

**Do not use `bind:this` + a `$effect` to focus** — use this action instead.

---

## 8. `inlineEdit` — Svelte Action for Inline Edit Inputs

**File:** `lib/actions.ts`

Encapsulates the commit-on-Enter/blur, cancel-on-Escape pattern for inline edit inputs. Prevents the classic **double-fire bug** where pressing Enter changes state that removes the input, causing blur to call the commit callback a second time.

```svelte
{#if editing}
  <input
    use:focusOnMount={true}
    bind:value={draft}
    use:inlineEdit={{ onCommit: () => save(), onCancel: () => revert() }}
  />
{/if}
```

**Behaviour:**
- **Enter** → calls `onCommit` once, then ignores the subsequent blur.
- **Escape** → calls `onCancel` once (with `stopPropagation`), then ignores the subsequent blur.
- **Blur** → calls `onCommit` (unless already committed/cancelled).

**Where used:** CardDetail block label rename, Sidebar hierarchy rename, Column rename, TagEditor tag rename.

**Do not** write inline `onkeydown` + `onblur` handlers for simple commit/cancel inputs — use this action instead.

---

## 9. SaveIndicator — Persistent Save Feedback

**File:** `components/SaveIndicator.svelte`

A small inline indicator that shows "Saving…" (orange, with spinner) while an API call is in flight, then flashes "Saved" (green, with checkmark) for 2.5 seconds after completion. Use this in editing dialogs where auto-save happens in the background and the user needs confidence their data persisted.

**Props:**

| Prop | Type | Default | Description |
|---|---|---|---|
| `saving` | `boolean` | `false` | Whether a save operation is currently in progress |

**Integration pattern — `tracked()` helper:**

```typescript
let savingCount = $state(0)
let saving = $derived(savingCount > 0)

async function tracked<T>(promise: Promise<T>): Promise<T> {
  savingCount++
  try { return await promise }
  finally { savingCount-- }
}

// Usage:
card = await tracked(UpdateCardTitle(cardId, title)) as Card
```

```svelte
<SaveIndicator {saving} />
```

**Where used:** CardDetail modal footer.

**i18n keys:** `common.saving`, `common.saved`

---

## 10. Card Type Design Tokens

**File:** `lib/cardTypes.ts`

Card type badge colours are centralised here. **Never** hardcode type colours inline in components.

```typescript
import { getCardTypeColor, getCardTypeTextColor } from '../lib/cardTypes'

// In a template:
style="background: {getCardTypeColor(card.type)}; color: {getCardTypeTextColor(card.type)}"
```

To add a new card type colour, add it to the `CARD_TYPE_COLORS` map in `lib/cardTypes.ts`.

---

## 11. LottiePlayer — Lottie / dotLottie Animations

**File:** `components/LottiePlayer.svelte`

A reusable wrapper around `@lottiefiles/dotlottie-web` for rendering `.lottie` (or `.json`) animations on a canvas. Use this anywhere a Lottie animation is shown — loading states, empty states, success flourishes.

**Props:**

| Prop | Type | Default | Description |
|---|---|---|---|
| `src` | `string` | — | URL to the `.lottie`/`.json` file. Import the asset with Vite's `?url` suffix: `import animation from '../lib/animations/x.lottie?url'`. |
| `loop` | `boolean` | `true` | Repeat indefinitely. |
| `autoplay` | `boolean` | `true` | Start playback as soon as the player is ready. |
| `ariaLabel` | `string` | — | Accessible label applied to the canvas (`role="img"`). Required for any animation conveying meaning. |
| `fallback` | `string` | `ariaLabel` | Visible text shown instead of the animation when the user prefers reduced motion. |
| `size` | `number` | `96` | Square canvas size in pixels. |

**Behaviour:**
- Respects `prefers-reduced-motion: reduce` reactively — falls back to the `fallback` text. No animation runs.
- The dotLottie WASM is served from `/dotlottie-player.wasm` (copied into `public/` by the `copy-vendor-assets` predev/prebuild script). No CDN — the app stays fully offline.
- Player is destroyed on unmount.

**Adding a new animation:**
1. Drop the `.lottie` file into `frontend/src/lib/animations/`.
2. Import via `?url` and pass to `LottiePlayer`.

**Example:**

```svelte
<script lang="ts">
  import LottiePlayer from './LottiePlayer.svelte'
  import loadingAnimation from '../lib/animations/loading.lottie?url'
  import { t } from '../lib/i18n.svelte'
</script>

<LottiePlayer
  src={loadingAnimation}
  ariaLabel={t('app.loading')}
  fallback={t('app.loading')}
  size={160}
/>
```

---

## 12. General Accessibility Guidelines

1. **All interactive elements must be keyboard-reachable.** Use `tabindex="0"` on non-button elements that act as buttons.
2. **Focus-visible must match hover state.** If a button turns red on hover, it must also turn red on `:focus-visible`.
3. **Escape always cancels** an in-progress edit without saving.
4. **Enter saves** in single-line edits; **Ctrl+Enter** saves in multiline edits.
5. **Use semantic HTML** — prefer `<button>` for actions, `<input>` for editable text.
6. **Never remove elements from the DOM** to hide them — use `color: transparent` or `opacity: 0` so they remain accessible to screen readers and keyboard navigation.

---

## 13. SidePanel, Workspace Panel & WorkspaceFileTree

**`SidePanel.svelte`** is the single right-hand panel host: one resizable, slide-animated container with a VS Code-style bottom tab bar, rendered as a sibling of `<Board/>` in App's `.board-row`. It owns ALL geometry (width persistence, drag-to-resize, slide in/out via width-not-transform animation — transforms break WebView2 scroll containers). Content components fill 100% and own zero geometry: `ChatSection` runs in `hosted` mode (its own shell/resize/animation disabled; card chat keeps `hosted=false`), `WorkspacePanel` is geometry-less by construction. Both tab panes stay mounted — inactive ones hide via CSS (`.sp-tab-pane.pane-hidden`) so chat drafts/scroll survive tab flips. TopBar buttons open-and-focus their tab, or close the panel when their tab is already frontmost; keyboard `p` = chat tab, `w` = workspace tab.

Future-layout note: SidePanel is the deliberate seam for a horizontal split or a generic drag-drop panel scheme — consumers pass `tabs` + a content snippet keyed by tab id; only SidePanel internals change.

**Panel-header convention** (Harvey, 2026-07-05): every panel header title is **icon + Proper Case** — never all-caps/`text-transform: uppercase`. Reference style: `0.82rem`, weight 600, `var(--text-strong)`, icon size 15, gap `0.4rem` (see WorkspacePanel's `.title` / ChatSection's `.chat-title`). Tab labels match their pane's header title verbatim.

The Workspace panel content (`components/workspace/WorkspacePanel.svelte`) renders inside a SidePanel tab. Sub-dialogs (attach, template editor, file viewer) are self-contained overlay components in `components/workspace/`.

**`WorkspaceFileTree.svelte`** is the reusable recursive collapsible tree:

| Prop | Type | Notes |
|---|---|---|
| `entries` | `WorkspaceEntry[]` | Flat, sorted, slash-relative paths (the whole tree; each instance filters its own level) |
| `prefix` | `string` | Path prefix this instance renders (`''` = root) |
| `onOpenFile` | `(path: string) => void` | File row click |
| `depth` | `number` | Indentation level (self-incremented on recursion) |

It self-imports for recursion (never `<svelte:self>` — deprecated). Keyboard: rows are real `<button>`s, so Tab/Enter work for free.

**Workspace file links**: markdown `workspace://<ws-id>/<path>` renders via `shared/markdown.ts` as `.bruv-link[data-workspace]`; the `main.ts` click interceptor dispatches `bruv:navigate {type: ''workspace-file''}` and App opens the panel + viewer. Same chain as `bruv:card:` links.
