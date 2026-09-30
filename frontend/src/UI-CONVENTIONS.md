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
| `rows` | `number` | `4` | Textarea row count (only used when `multiline` is true) |
| `class` | `string` | `''` | Extra CSS class applied to the root element |
| `onSave` | `(value: string) => void` | — | Called when the user commits a change |
| `onCancel` | `() => void` | — | Called when the user cancels (Escape) |
| `onTab` | `() => void` | — | Called on Tab keypress — useful for moving focus to the next field |

**Keyboard behaviour** (implements the Keyboard Entry Contract — §8):
- **Click** or **Enter/Space** (when focused on the display) → enters edit mode, selects all text.
- **Enter** → saves in BOTH modes; in multiline mode **Shift+Enter** inserts a newline.
- **Ctrl+Enter** → saves and closes the containing card/dialog (via its `EditScope`).
- **Escape** → cancels edit, reverts to original value, calls `onCancel`.
- **Tab** → saves then calls `onTab` if provided (single-line only).
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

**Add-item input** at the bottom is a *serial* input per the Keyboard Entry Contract (§8): Enter adds the item and re-arms for the next, Escape discards the draft and ends the entry, blur keeps the draft uncommitted.

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

## 4.5 `.btn` — the standard app button (GLOBAL)

`.btn` lives in **`style.css`**, not in each component. Modifiers: `.primary` (accent fill), `.subtle` (quiet third action). Sizing, padding and hover are the app's, once.

**Why global matters here:** Svelte scopes component styles, so a `.btn` rendered inside a **child** component gets none of the parent's rules and comes out unstyled. That is exactly what happened when the Workspace panel's local-copy section and the remote-attach step were extracted into their own components (2026-08-02) — the buttons rendered as bare browser buttons. Any future extraction would hit it identically, which is the opposite of what extracting a component is for.

A component with a genuinely different button still wins: Svelte adds a scoping class, so a local `.btn` outranks the global one. Treat the global as the default, not a lock — but don't re-declare it just to restate the same values (the workspace dialogs and panel had four near-identical copies; they're gone).

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

**Keyboard (2026-09-30):** Escape cancels, click-outside cancels. **Enter activates the focused button** — Tab to Cancel + Enter cancels; with focus anywhere else (the dialog, the page underneath) Enter confirms. Ctrl/Cmd+Enter follows the same rule (it used to confirm blindly). The dialog owns these keys through the key-layer stack (§8.1): capture phase, consumed, so a confirm raised from inside a card never lets its answering Escape/Enter reach the card — that Escape used to close the card too, and on a fresh "New Card" delete it.

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

## 8. Keyboard Entry Contract — `inlineEdit` action + `EditScope`

**Files:** `shared/inlineEdit.ts` + `shared/editScope.ts` (desktop re-exports the action from `lib/actions.ts`). **Applies to BOTH surfaces.**

Every data-entry surface follows ONE contract (ruling, 2026-07-10):

| Key | Behaviour |
|---|---|
| **Enter** | Commits and ends the edit. Multiline: **Shift+Enter** inserts a newline — chat-style everywhere, including card description, text blocks, and comments. Serial "add another" inputs (checklist/list/media/option add rows): commits the item and re-arms for the next. |
| **Escape** | Cancels without committing (revert draft). Consumed (`preventDefault` + `stopPropagation`) — never bubbles while an edit is active. |
| **Ctrl/Cmd+Enter** | Commits, then closes the containing card/dialog. Sole exception: the chat composer sends **without** closing (`closeOnCtrlEnter: false`). |
| **Escape, nothing editing** | Closes the card/dialog/sheet. |
| **Blur** | Commits edit-in-place fields. Add-inputs and composers keep their draft uncommitted — never silently discard. |

Discrete pickers (select/date/rating/checkbox/radio/color/icon) commit on choice; Escape closes them unchosen. They are not draft-based.

**Mobile-surface variant (ruling, 2026-07-10).** The table above assumes a hardware keyboard. The mobile PWA is touch-first, so per-surface (no input-modality sniffing):
- Single-line fields: virtual Enter commits, and every field declares `enterkeyhint` so the keyboard's action key says what it does.
- **Multiline fields: virtual Enter inserts a newline** (platform-native — Shift+Enter needs two fingers on a touch keyboard). Commit = tap-away (blur) or the explicit ✓ Done button on the active editor. Chat: Enter = newline, the Send button sends. Hardware chords still work everywhere: Ctrl+Enter commits (+closes), Escape cancels. Implemented via the `enterInsertsNewline` option on `inlineEdit`/`draftEdit` — desktop never sets it. **Multiline markdown fields are tap-to-edit (ruling 2026-09-21):** the description and every text block render prose at rest, open the editor on tap, and return to prose on ✓ Done / tap-away — one component, `EditableDescription`, for both. No always-on editor, no edit/preview toggle: the rendered markdown is the preview.
- **Back = Escape**: while an edit is active, Back (app ← button, gesture, hardware) cancels the edit and the page stays open; with nothing active it navigates. Composers (chat, quick capture, comments) preserve their draft on a Back-cancel — an accidental back-swipe must never eat a typed message. A cancel (or commit) never moves the page's scroll position — the mobile router sets `history.scrollRestoration = 'manual'` so the undone traversal can't re-apply a stale scroll offset.
- **Checklist/list item rows are multiline editors (ruling, 2026-09-20).** Editing a long item in a single-line input on a phone scrolled the text out of view and made the caret impossible to place, so `EditableItemText` now edits in an auto-growing, wrapping textarea (`autoGrow` with `minHeight: 0`, so a short item starts one line tall) and follows the multiline contract above: virtual Enter inserts a newline, tap-away or the row's ✓ Done commits, Ctrl+Enter commits and closes, Escape/Back cancels. Rows top-align their handle/tick/bullet to the first line. This supersedes the 2026-07-10 "Enter just commits" rule; there is still no next-row advance — adding rows is the + button's job (it spawns a blank auto-edit row). Desktop's serial add-input keeps its re-arm behaviour — mobile-surface rule only.
- **Desktop list/checklist rows are multiline too (2026-09-25).** Item rows edit in `EditableText` with `multiline grow` (a one-line textarea that wraps and grows via the shared `autoGrow` action, ~8 lines then scrolls), and the add row is the same auto-growing textarea. They keep the desktop contract: Enter commits (the add row re-arms), Shift+Enter inserts a newline, Tab still advances to the row's delete button. Rows top-align handle / tick / bullet to the first line and render stored newlines (`white-space: pre-wrap`).
- **Cancelling a just-added blank row removes it** (add-cancel per §12.5): Back, hardware Escape, or tap-away with an empty draft on a row that never committed text leaves nothing behind (`EditableItemText` fires `onEmpty` from its cancel path when the committed text is blank).

**Field side — the `inlineEdit` action.**

```svelte
{#if editing}
  <input
    use:focusOnMount={true}
    bind:value={draft}
    use:inlineEdit={{ onCommit: () => save(), onCancel: () => revert(), scope: editScope }}
  />
{/if}
```

Options: `onCommit`, `onCancel`, `multiline`, `serial`, `blurCommits` (default true; forced off by `serial`), `container` (CSS selector — blur ignored while focus stays inside it), `scope`, `closeOnCtrlEnter`. The action also prevents the classic **double-fire bug** (Enter unmounts the input → blur would commit a second time) and ignores keystrokes during IME composition.

Hand-rolled handlers are acceptable ONLY where the flow is genuinely special (suggestion pickers; mobile item rows where Enter must commit without closing the page) — they must still implement the table above and register with the scope. Reference: `CardTagsField.svelte`.

**Always-mounted fields inside a card** (media captions, an empty image block's url/caption pair) still use `inlineEdit` + the scope, but with `serial: true` so they register only while focused/typing — a non-serial field registers on mount, and an idle one would hold the scope open so Escape could never close the card. A caption that should also commit on blur adds that explicitly (`MediaBlock`). Bypassing the action (hand-rolled Enter, `onchange`-only commits) let Escape close the card and drop the typed caption.

**Container side — `EditScope`.** Each closable container (card dialog, modal dialog, mobile sheet) creates a scope, sets `requestClose`, and shares it via `setContext(EDIT_SCOPE_KEY, scope)`. Nested dialogs create their own scope — context shadowing routes fields to the nearest container. The container's window keydown asks `scope.hasActive()` before closing on Escape and calls `scope.commitAll()` for the Ctrl+Enter chord (`scope.handleWindowKeydown` is a ready-made helper). The scope also feeds "don't clobber my edit" guards (e.g. CardDetail skips silent card reloads while the scope has active edits).

**Do not** write inline `onkeydown` + `onblur` handlers for simple commit/cancel inputs — use this action.

### 8.1 Layered dialogs — shield BOTH window keys (recurring data-loss bug)

Containers like CardDetail own **two** window-level keys: Escape (close when nothing edits) and **Ctrl+Enter (commit-all + close)**. Any dialog/sheet/picker layered above such a container MUST intercept **both** — every Escape-only shield has later resurfaced as a Ctrl+Enter bug that closed the card underneath and discarded the layer's edits (SlideEditorDialog hit this twice: Escape 2026-07-18, Ctrl+Enter 2026-07-27).

In the layer, route the keys per the §8 table *scoped to the layer itself*: Escape cancels/closes the topmost thing (open dropdown first, then the layer), Ctrl+Enter commits **the layer's surface**, not the card's.

Two working shield patterns — pick by DOM relationship to the parent's listener:

1. **Child-side capture shield** — a capture-phase window listener (`window.addEventListener('keydown', h, true)`) registered while the layer is open; `preventDefault()` + `stopPropagation()` stops the parent's bubble-phase `<svelte:window>` handler. Reference: `SlideEditorDialog.handleCaptureKeydown`, mobile `ChatSheet`.
2. **Parent-side stand-down guard** — required when the layer is a portaled **sibling** that itself listens on `<svelte:window>`: `stopPropagation()` cannot stop sibling listeners on the same node (only `stopImmediatePropagation`, unavailable from `<svelte:window>`). The parent checks the layer's visibility and returns early. Reference: CardDetail's `optionsEditorState.visible` / `showCreateTypeDialog` / `showPromoteDialog` guards.

3. **Key-layer stack — `lib/keyLayer.ts` (preferred, 2026-09-30).** One ordered stack with a single capture-phase window listener: Escape and Enter go to the **topmost** layer only and a handled key is consumed, so nothing underneath — the card's bubble-phase handler, a lower layer's capture listener — sees it. Several raw capture listeners on `window` all fire in *registration* order (lowest layer first), which is why patterns 1–2 kept leaking. Attach `use:keyLayer={{ onEscape, onEnter? }}` to the overlay element that only mounts while open (or `pushKeyLayer(...)` in an `$effect` for conditional state). Without `onEnter`, plain Enter is left to the focused control and the Ctrl/Cmd+Enter chord is **swallowed** (it must never commit-and-close the card); a handler returning `false` declines and the key falls through to the next layer. Users: `ConfirmDialog`, `CardShareMenu`, `BlockPicker`, `PinPicker`, the card type picker, the block promote menu, the attachment preview, `SlideEditorDialog` (+ its `BoundField` / `MediaRefList` dropdowns stacked above it).

   The same module counts open modals (`use:modalOpen` on CardDetail's backdrop); `globalShortcutsBlocked()` is true while a modal or a layer is open, and App's single-key shortcuts (`p` `w` `/` `?`) stand down — they used to open panels behind an open card.

Patterns 1–2 remain in older dialogs (OptionsEditorDialog, CreateTypeFromCardDialog, PromoteCardDialog use pattern 2); migrate them to the stack when touched. Every new layered dialog/dropdown uses the stack — for both keys — or it ships this bug again.

### 8.2 Card saves — one block-save queue, close only after commits land

- **Every whole-blocks write for the open card goes through ONE serialized queue** (`lib/cardBlockSaves.svelte.ts`, built on `shared/serialSave.ts`), created per card in CardDetail and handed down as `saveBlocks` (CardBlocks → BlockItem). One persist in flight; later writes coalesce to the latest snapshot. Edits apply to the local card first (value edits in place via BlockItem `commitBlock`, structural changes via CardBlocks `persistBlocks`) and roll back if the save fails and nothing newer replaced them. Never call `UpdateCardBlocks` directly from a card component.
- **Silent reloads never clobber pending saves:** a `card:updated` (including the watcher's ~200 ms echo of our own save) while the queue is busy is deferred and run once after it drains; a reload whose response lands after a save was requested is discarded and retried. Root cause of "rapid checklist checks lose items".
- **Ctrl+Enter closes only when every commit succeeded.** Title/description/text-block saves resolve a success boolean, and the scope's `requestClose` waits for every tracked save (`closeWhenSaved`); any failure keeps the card open with the draft and the save's toast.
- **Fresh-card cleanup (Board):** Escape deletes a just-created card only while it is pristine — the fresh flag (`freshCardId`) is cleared by *any* `onUpdated` from the dialog: title, blocks, tags, comments (`CardComments.onChanged`), attachments, agent config (`AgentTab.onChanged`), chat edits, pins, JSON merges. A silent reload never re-opens the auto title editor.
- **Mobile equivalent (2026-09-30):** CardPage's block saves go through `mobile/src/lib/cardSaveQueue.ts` (also on `shared/serialSave.ts`, plus the 200 ms debounce). Quiet refetches are gated on its edit counter (`canApplyRefetch(since)`) — a fetched card is discarded if any local edit happened since the fetch began or a save is pending, and one refresh runs after the queue drains. Leaving the page (`leave()`) flushes a pending debounce instead of dropping it; a failure after the page is gone toasts `card.err_save_on_leave`. Attachment uploads merge only `file_attachments` back into the card, never the whole object. _Follow-up: the desktop and mobile queues are near-twins and should fold into one `shared/` helper._

**Mobile ConfirmDialog (2026-09-30):** destructive dialogs focus **Cancel** on open (non-destructive ones the primary action), and the dialog owns Enter and Ctrl+Enter in the capture phase — they activate the focused button (confirm when focus is elsewhere) and never reach the page underneath, so Ctrl+Enter can't close a card behind an open confirm.

**Mobile DocumentSheet (2026-09-30):** saves are serialized (each save presents the stamp its predecessor wrote — no false "changed on disk" prompt for your own edit). A save that fails without divergence offers "Discard unsaved changes?" so an offline user can still close; a refused close (Back while the draft can't be saved) pushes the sheet's history entry back so the next Back doesn't silently leave the card.

**Mobile sheet chrome (2026-07-31):** new mobile bottom sheets use
`mobile/src/components/BottomSheet.svelte` (props: `title`, `subtitle?`,
`historyKey`, `onClose`, children) — it owns the backdrop, fly-in, and the
three dismissal paths (backdrop tap / Escape / Android Back via a history
entry) so sheets don't hand-roll them. First users: the clip flow's
DeckTargetPicker and ClipPinPicker. Pre-existing sheets (PinPicker,
CardTypePicker, …) migrate opportunistically, not in bulk.

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

## 9.5 `clickOutside` Action & Shared Dropdown Chrome

**Files:** `lib/actions.ts` (`clickOutside`), `style.css` (`.dropdown-menu` family)

`clickOutside(node, { onOutsideClick, exclude? })` closes popovers/menus on any click outside the node. Pass the trigger element in `exclude` so its own click can toggle without immediately re-closing. Attach to the popover content and pair with conditional rendering so it only listens while open. **Do not** hand-roll document-level click listeners for this — four divergent copies were consolidated into this action (2026-07-10).

Dropdown menus share the global `.dropdown-menu` / `.dropdown-menu-item` classes (+ `.dropdown-menu--grid` for grid layouts) in `style.css` — same shared-utility pattern as `.action-reveal`. Discrete dropdowns also close on **Escape** (consumed, via `use:keyLayer` — §8.1 — never a `<svelte:window>` handler, which fires after the card's and closed the card too) per §8/§12.5, and every `:hover` style needs its `:focus-visible` twin (§12.2). Reference implementations: `CardShareMenu.svelte`, `BlockPicker.svelte`.

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
3. **Escape always cancels** an in-progress edit without saving — and closes the containing card/dialog only when nothing is being edited (see §8).
4. **Enter commits** in ALL edits (multiline uses Shift+Enter for newlines); **Ctrl+Enter** commits and closes the container (see §8).
5. **Use semantic HTML** — prefer `<button>` for actions, `<input>` for editable text.
6. **Never remove elements from the DOM** to hide them — use `color: transparent` or `opacity: 0` so they remain accessible to screen readers and keyboard navigation.

---

## 12.5 Drag Surfaces & Grip Handles

The design metaphor is **"everything you expect to be draggable is"** — draggable elements get `cursor: grab` and body-drag, with **no grip icon** (sidebar tree, kanban columns, cards, blocks-as-a-whole).

**Sanctioned exception (ruling, 2026-07-10):** rows whose *body is an edit surface* — clicking the row starts editing or another primary action — keep a `GripVertical` as their only drag surface, because the click has to mean "edit": BlockItem, EditableChecklist/EditableList item rows, SurveyBlock questions, OptionsEditorDialog/TemplateEditor/TemplateEditorDialog option & param rows, LLMProvidersManager provider rows and RoutingRuleRow rule rows (click expands the inline editor), MCPServersDialog arg rows (text inputs), and mobile Checklist/ListBlock (where the grip also avoids the touch-scroll conflict). Do not "fix" grip-only dragging on such rows, and do not add grips anywhere else.

**Mobile touch drag (`mobile/src/lib/actions/dnd.svelte.ts`):** long-press arms, a short tap passes through, so every body-drag surface above also drags on the phone: cards (with the category body as drop target), brand › stream › project rows in the Browse tree, and, since 2026-09-20, **category rows on the Project page** (long-press the category name; bodies collapse for the drag; persists via `ReorderCategories`). Nest one `dragSortable` per level, each with its own `rowSelector` / `dropTargetSelector`: the inner instance stops propagation for presses on its rows, so outer levels never arm on them. Use `handleSelector` to keep row buttons and rename inputs out of the pickup area.

**Invisible AI actions (ruling, 2026-09-20):** an action that hands work to the AI on the user's behalf — quick capture's *Create with AI* — never opens the chat. It creates the object first (the note is an Inbox card before the model is involved), runs the turn in forced edit mode through a dedicated RPC (`PopulateCardWithAI`, `chat.SendCardEdit`) so the global Suggest/Chat setting can't leave the result half-applied, keeps the exchange in the card's chat history, and drops the user onto the object with a status banner while the edits stream in over `card:updated`. The in-flight state lives in a module (`mobile/src/lib/aiCreate.svelte.ts`), not in the sheet that started it, because the sheet closes immediately. Failure = toast; the object stays.

**Suggest-mode edit outcomes (ruling, 2026-09-25):** a staged edit has four states and each is visually distinct: *pending* (checkbox), *accepted* (green tick), *rejected* (grey cross, struck through), *failed* (red cross, row tinted, the refusal reason on hover / long-press via `chat.edit_failed_reason`; the label still shows what was proposed). Applying a batch is one call that **always returns the resolved rows** — a tool refusing one edit (a category that does not accept the card's type, say) marks that row `failed` with `error` and never turns the call into a transport error, because that would hide the other rows' outcomes; the surface then toasts the failed count (`error.edit_apply_some_failed` / `chat.err_apply_some`). Failed is terminal: the user fixes the cause and asks the AI again. Never render an edit as accepted on the strength of the click alone — the tick comes from the backend's status. Backend: `resolvePendingEdit` in `core/supervisor/runtime_methods.go`; rows: desktop `ChatSection`, mobile `chat/ChatMessage`.

**Delete vs add-cancel (ruling, 2026-07-10):** clicking a **delete/clear button always confirms** via ConfirmDialog — even for empty containers and zero-usage tags. Cancelling an add-flow (Escape on an untouched placeholder, backing out of an add input) **never prompts**.

The boundary (Harvey, 2026-07-10): confirmation applies to deleting an **object** (card, category, brand/stream/project, tag, template, agent, attachment, comment, notification list). Removing a **row inside an editing surface** — a checklist/list item, media item, tag chip on a card, select option, survey question, MCP arg, template param, a single notification — is an *edit to the containing object*, not a delete, and stays promptless.

**Create-then-rename (ruling, 2026-08-09):** a create action that drops the user straight into a name prompt must leave **no object behind on cancel**. Escape — or system Back on mobile, which is the same key per §8 — deletes the fresh object **unconditionally**, whether or not the user typed into the draft first; a cancelled create never survives with its default name. Flows that show an input *first* (quick capture, tag add, deck create) satisfy the rule by creating nothing until commit. The §8 "cancelling a just-added blank row removes it" rule is the row-level special case of this. Reference implementations: mobile `BrowsePage` brand/stream/project create (unconditional `silentDelete` from `cancelRename`); desktop `Sidebar`/`Board` fresh-create cancels (same shape, gate removed 2026-08-09). Create flows whose name prompt lives on a **page** (not a focused dialog) also need a window-level Escape fallback and a Back-cancels-rename popstate intercept — the shared `inlineEdit` action's Escape is node-scoped and mobile WebViews may refuse the programmatic focus (see ProjectPage's category rename for the pattern).

---

## 12.6 Stepper Navigation with Graceful Fallback

Harvey's pattern (2026-08-14, chat bookmarks — bottled by request): when a
"jump to next/previous X" control would be a dead button because no X exists
in that direction, it falls back to the nearest meaningful boundary instead
of no-oping. The chat panel's bookmark chevrons jump between bookmarked
questions and fall through to the first/last question when no bookmark
remains — so with zero bookmarks the same two buttons are jump-to-start/end.
Three buttons, five jobs, nothing to explain. Prefer this shape for any
future stepper over disabled states or extra button pairs.

## 12.7 Action-Button Labels

**Short verb labels; tooltips carry the explanation (ruling, 2026-07-17).** Action buttons are labelled with the bare verb — "Delete", "Share", "Promote" — not verb + object ("Delete Card"). The surrounding context already names the object; the `title` tooltip holds the longer explanatory text (e.g. the card footer's delete button: `common.delete` label + `tooltip.delete_card` = "Delete this card permanently").

Bare-verb labels use the shared keys `common.add` / `common.delete` / `common.remove` / `common.save` (both surfaces) — don't mint per-context keys for a value that is just the verb. A per-context key is only warranted when the label genuinely differs (e.g. "Delete All").

## 12.8 Wide Row Buttons Opt Out of the Press Pulse (`press-still`)

`style.css` gives every `button:active` a 140 ms press pulse (`scale` to 0.94 and back). On a compact button that reads as feedback. On a **full-width row button** — a tree row, a list row, anything `flex: 1` — a 6 % shrink moves the row's edges inward by several pixels, so a quick click on something at the edge (the chevron) ends with the pointer *outside* the button and the browser never fires `click`. Holding the mouse longer "fixes" it because the pulse completes before mouse-up. Field report 2026-09-17 on the Workspace Files block; the project tree had shown the same before.

**Rule:** a wide row button whose click target sits at an edge carries `class="press-still"`, which cancels the pulse for that element only. Applied to `WorkspaceFileTree` rows, `WorkspaceFileEntryRow`, `WorkspaceFilesTreeView` implied folders, and the Sidebar's brand/stream rows. The mobile surface has no press pulse, so nothing to opt out of there.

**Corollary — never swap the element under the pointer on toggle.** Expand chevrons are one `<span class="chev">` holding a single `ChevronRight`, rotated 90° with a CSS transition when open, rather than an `{#if}` that replaces `ChevronRight` with `ChevronDown`. Replacing the icon mid-gesture is a second way to lose the click, and a rotation is the better animation anyway.

---

## 13. SidePanel, Workspace Panel & WorkspaceFileTree

**`SidePanel.svelte`** is the single right-hand panel host: one resizable, slide-animated container with a VS Code-style bottom tab bar, rendered as a sibling of `<Board/>` in App's `.board-row`. It owns ALL geometry (width persistence, drag-to-resize, slide in/out via width-not-transform animation — transforms break WebView2 scroll containers). Content components fill 100% and own zero geometry: `ChatSection` runs in `hosted` mode (its own shell/resize/animation disabled; card chat keeps `hosted=false`), `WorkspacePanel` is geometry-less by construction. Both tab panes stay mounted — inactive ones hide via CSS (`.sp-tab-pane.pane-hidden`) so chat drafts/scroll survive tab flips. TopBar buttons open-and-focus their tab, or close the panel when their tab is already frontmost; keyboard `p` = chat tab, `w` = workspace tab.

Future-layout note: SidePanel is the deliberate seam for a horizontal split or a generic drag-drop panel scheme — consumers pass `tabs` + a content snippet keyed by tab id; only SidePanel internals change.

**Panel-header convention** (Harvey, 2026-07-05): every panel header title is **icon + Proper Case** — never all-caps/`text-transform: uppercase`. Reference style: `0.82rem`, weight 600, `var(--text-strong)`, icon size 15, gap `0.4rem` (see WorkspacePanel's `.title` / ChatSection's `.chat-title`). Tab labels match their pane's header title verbatim.

The Workspace panel content (`components/workspace/WorkspacePanel.svelte`) renders inside a SidePanel tab. Sub-dialogs (attach, template editor, file viewer, new file/folder, from-template, file picker) are self-contained overlay components in `components/workspace/`.

**Panel sections, in order (ruled 2026-09-17, `plan/2026-09-17 workspace files block.md` §5):** **Files** — the tree plus the structure actions (`WorkspaceStructureActions`: New file / New folder / From template, on the root from the section header and on any folder from its hover-revealed row actions); **On this device** (`WorkspaceDeviceSection`, collapsed by default) — open folder, launch command, this device's clone (`WorkspaceLocalCopy`), and the commit-on-save toggle for a published workspace; **Details** (collapsed by default) — adapter, origin, warnings. The split exists so editing never reads as part of cloning: **the editor edits the host copy over the connection; a clone is only for other apps on this device.** A created file opens straight into the editor; a created folder expands to it.

**`WorkspaceFileTree.svelte`** is the reusable recursive collapsible tree. It is **lazy**: one instance renders one directory level, and mounting it is what loads that directory.

| Prop | Type | Notes |
|---|---|---|
| `cache` | `WorkspaceDirCache` | Per-directory cache from `lib/workspaceTree.svelte.ts` (`createWorkspaceDirCache(loader)`), created ONCE by the root consumer |
| `dir` | `string` | Directory this instance renders the children of (`''` = root) |
| `onOpenFile` | `(path: string) => void` | File row click |
| `depth` | `number` | Indentation level (self-incremented on recursion) |
| `collapsed` | `Record<string, boolean>` | Shared expand state owned by the root consumer — what makes Expand/Collapse All and accordion mode global. **A folder is expanded only when `collapsed[path] === false`; absent = collapsed**, so the tree opens closed |
| `mode` | `'single' \| 'multi'` | `single` = accordion (expanding collapses siblings) |
| `workspaceId` | `string?` | When set, rows drag out as Workspace Files entries (`lib/workspaceDrag.ts`, custom `dataTransfer` type `application/x-bruv-workspace-entry` — the one cross-component drag in the app) |
| `onCreateIn` | `(dir, kind: 'file' \| 'dir' \| 'template') => void` | Shows New file / New folder / From template on folder rows (hover/focus-revealed) |
| `selected` + `onToggleSelect` | `Record<string, boolean>`, `(node) => void` | **Picker mode**: every row gets a checkbox and a file tap toggles instead of opening. Same shared-record ownership rule as `collapsed` |

**Never load the whole tree.** Rendering from one whole-workspace index made opening the panel cost everything on disk — one `node_modules`, a nested `.git`, or a folder of 80k photos anywhere under the root choked it. A server-side blocklist of heavy directory names was rejected (Harvey, 2026-08-02): *"a band-aid, not a solution — this issue would apply to a .git folder or many others neither you nor I can predict. The tree should load collapsed and load incrementally."* So the tree opens **collapsed**, and expanding a folder calls **`ListWorkspaceDir(brand, stream, project, rel)`** for that one folder. A directory nobody opens is never read.

`createWorkspaceDirCache(loader)` is the store: keyed by directory path (`''` = root), each entry a `WorkspaceDirState` union — `{status:'loading'}` / `{status:'ready', children, isTemplateRoot}` / `{status:'error', error}`. `ensure(dir)` fetches on a miss and is a no-op on ANY cached entry, so collapse + re-expand never refetches and a failed folder keeps its error until retried; `reload(dir)` is the retry; `clear()` drops everything and discards in-flight responses (generation counter) so a Refresh can't be overwritten by a stale listing. Sibling order is the server's (path-ascending, files and folders interleaved — *not* folders-first).

**Three distinct states per level, never conflated**: `common.loading` row while in flight, a `workspace.dir_failed` row with a `common.retry` button on failure, and *nothing at all* for a genuinely empty directory. A folder that failed to list must never render as empty.

**Expand All is bounded.** "Expand everything" would re-walk the whole workspace — the exact cost lazy loading exists to remove. The button expands only directories already in the cache and fetches nothing, and is labelled `workspace.expand_loaded` ("Expand loaded folders") rather than `sidebar.expandAll` so the label doesn't promise the tree. Collapse All stays global and is just `treeCollapsed = {}` (absent = collapsed).

**Folder-Template roots** get the cyan `LayoutTemplate` icon when *known*: lazily, a directory is only classifiable once its own children are loaded, so the rule is "a loaded directory whose children include a `.ft` folder". The icon appears when the folder is opened; nothing blocks on it.

**Built-in Folder Templates** (Screenplay, Manuscript) are embedded in the binary under `core/services/workspace/builtin_templates/<Name>/{title}/` and **seeded once** into `<vault>/templates/<Name>/` on repo load (`Service.SeedBuiltinTemplates`, marker `templates/.bruv-builtin.json`, which travels with the vault). Never overwritten: a user folder of the same name wins, and a deleted seed stays deleted. A new built-in is a new folder there — nothing else to register.

`WorkspacePanel` owns the cache and calls `resetTree()` (clear + drop expand state) on Refresh and on project change; mounted levels re-fetch themselves because each level's `$effect` reads its cache entry and reloads when it goes missing — so a refresh costs what's on screen, not the tree.

It self-imports for recursion (never `<svelte:self>` — deprecated). Keyboard: rows are real `<button>`s, so Tab/Enter work for free.

**`WorkspaceLocalCopy.svelte`** — this device's working copy, rendered inside the Workspace panel **only on a remote connection** (`isLocalActive()` is false). On a remote connection the workspace's files are on the server, so Tier 1 actions have nothing local to act on until the device clones a copy. The component owns that whole lifecycle: report → confirm → the host publishes (`Workspace.git_serve`: `initializing` → `ready`/`error`, vault-side so every client sees it) → this device clones (shell-side status, polled ONLY while a clone runs) → pull/push/forget.

| Prop | Type | Notes |
|---|---|---|
| `ws` | `Workspace` | Carries `git_serve` — the host-side half of the lifecycle |
| `brandSlug` / `streamSlug` / `projectSlug` | `string` | Project coordinates for the publish RPCs |
| `serverName` | `string` | `activeConnectionLabel()` — every message names the machine |
| `onCheckoutChange` | `(info: WorkspaceCheckoutInfo \| null) => void` | Lets the panel point Tier 1 actions at the clone |

**Two machines means saying which one.** Every string in this section interpolates `{server}` — "Preparing the workspace on HOMEBOX", "git isn't installed on HOMEBOX". The bug this feature fixes was an error that read as "your folder is missing" when the folder was on the user's own disk and the *server* couldn't see it. Applies to the attach error too, which now names the machine it searched.

**Tier 1 actions follow the files, and hide when there are none.** `WorkspacePanel` derives `deviceRoot` — the origin path when the vault is served from this machine, otherwise the clone's path, `undefined` when neither. Open-folder and the launch command are hidden rather than offered-and-failing.

**`WorkspaceFilesBlock.svelte`** (+ `WorkspaceFilesTreeView`, `WorkspaceFileEntryRow`, `WorkspaceFilePickerDialog`) — the **Workspace Files** block type (`workspace_files`): the files and folders a card is about, opened in the document editor from the card. Value = `WorkspaceFileEntry[]` (`{ id, workspace_id, path, is_dir? }`, path workspace-relative), `meta.display` = `'tree' | 'flat'`. Shared helpers in `shared/workspaceFiles.ts` (`buildWorkspaceFilesTree`, `mergeWorkspaceFiles`, `newWorkspaceFileEntry`) and `shared/blockValues.ts` (`asWorkspaceFiles`). Rules: an entry carries its own workspace id and resolves through `lib/workspaceLocations.svelte.ts` (`ResolveWorkspace`, cached per session; one lazy dir cache per workspace shared by every block), so the block works wherever the card renders, Inbox included; an empty block finds its workspace through the card's pins (`GetCardPinBreadcrumbs` → `GetWorkspaceState`); a folder entry expands into the live subtree through `WorkspaceFileTree`; a path that no longer resolves (checked against its parent's listing — never on a guess) shows **Missing** with **Relink**, because paths are the identity and BRUV does not track renames made outside it; add = the picker, which IS `WorkspaceFileTree` in selection mode with the structure actions (a file created from a card is selected straight away); drop target for tree drags. Named **Workspace Files** in the add-block menu, never Files, so it is not mistaken for Attachments. Card chat gets `read_card_file` scoped to exactly these entries (`core/runtime/tools/card_files.go`). Replaced Card Folders (2026-09-17): a legacy `card.folder` loads as a block with one folder entry (`internal/repo/card.go` migration).

**`ServerFolderStep.svelte`** is the remote half of `AttachWorkspaceDialog`: the native picker browses THIS disk, but `AttachWorkspace` opens the path on the machine running the vault, so on a remote connection the dialog asks the user to type a path the server knows. Gate on `isLocalActive()`, **not** `capabilities.hasLocalFilesystem` — that one is set from `wailsShellAvailable` and answers "is a Wails shell present", which is true on the desktop against every connection.

**`lib/format.ts` — `formatBytes(bytes)`** is the one way BRUV writes a size the user is asked to agree to (template import, published-workspace initial commit). Whole units up to 100, one decimal where it carries meaning ("1.4 GB", "150 MB"). Use it rather than dividing by 1024 at the call site.

**Workspace file links**: markdown `workspace://<ws-id>/<path>` renders via `shared/markdown.ts` as `.bruv-link[data-workspace]`; the `main.ts` click interceptor dispatches `bruv:navigate {type: ''workspace-file''}` and App opens the panel + viewer. Same chain as `bruv:card:` links.

---

## 14. Slide Template Auto-Matching (templatePrefs store + Settings tab)

Slides whose `templateId` is `'auto'` (what the clipper stamps) resolve their
template at render time: `resolveSlideTemplate(templateId, contentTypeId,
captureUrl, prefs)` in `shared/slideTemplates.ts` matches each template's
`urlHint` regex against the slide's capture URL (`values.url`,
post-binding-resolution). An explicit template pin ALWAYS wins — hints power
Auto only and never filter the manual picker (ruling, 2026-07-31; rendering
Facebook content on the X template is a choice, not an error).

**`lib/templatePrefs.svelte.ts`** is the shared reactive store for the
vault-level prefs (`GetTemplatePrefs`/`SetTemplatePrefs`): Auto priority
`order` + per-template `urlOverrides`. Consumers call `loadTemplatePrefs()`
on mount/open and pass `templatePrefs()` into `SlideRenderer` (prop-injected
— the renderer stays surface-agnostic).

**Auto in the picker (SlideEditorDialog):** when the slide has a capture URL
the template seg-picker gains an "Auto (X Post)"-style first entry showing
the LIVE resolution; the label updates when a new template's hint starts
matching. Auto survives content-type switches; invalid explicit pins reset.

**`SlideTemplatePrefsSection.svelte`** (Settings → Slide Templates tab)
lists the hint-carrying templates: rows drag to set multi-match priority
(row body drags, no grips — §12.5), pattern inputs override the built-in
regex with inline invalid-pattern errors (an uncompilable override falls
back to the built-in hint at render time — rendering can never break), and
"Reset" clears an override promptless (an edit, not a delete — §12.5
boundary). The section persists itself on commit; the dialog's Save is not
involved.

`embed://<provider>/<id>` media values (YouTube captures) render as the
platform's official iframe player in both renderers (`.pc-embed`/`.r-embed`)
— allowlist-parsed, unknown providers render nothing.

**Slide overflow (ruling, 2026-07-31):** captured content can't be authored
shorter, so every slide has an `overflow` option — **Scale to fit** (default)
or **Scroll**. Fit mode shrinks text step-wise (14px floor) and then
transform-scales the `.frame-fit` / `.fit` inner wrapper as the backstop: a
slide can NEVER clip. The scale lives on the inner wrapper, never the
animated frame (entrance keyframes own the frame's transform). Scroll mode
renders full-size: desktop/editor stages get a themed scrollbar; `/present`
pages via the console's within-slide ⬆/⬇ pair (below). `SlideRenderer`
reports overflow via `onOverflowChange`; the editor preview shows a
"Scaled to fit"/"Scrolls" badge.

**Gallery carousels (2026-07-31):** a media (image) field whose resolved
value contains multiple newline-joined URLs renders as a carousel — one
image visible, counter badge. This is schema-free: the `post` content type
keeps its single `media` field; multi-image cards store a multi-item
`media` block, and binding resolution (`slideBindings.ts` + present.go's
mirror, which also signs attachment refs line-by-line) joins every item
URL for image fields (video fields stay first-URL). Desktop / editor
preview: the image itself is the advance button (no chrome, always
"next"). On `/present`: the within-slide ⬆/⬇ pair steps the carousel.

**Within-slide ⬆/⬇ axis (ruling, 2026-07-31 — supersedes the one-way ⬇ /
Images buttons):** scroll paging and carousel stepping are the same
concept — a dimension WITHIN the current slide — so the console shows ONE
standard control pair (`ChevronUp`/`ChevronDown`; vertical = within the
slide, horizontal ◀▶ = between slides) whenever the current slide has a
within-slide dimension. It drives scroll on scroll-mode slides (clamped
at both ends — the old wrap-at-bottom existed because a one-way press had
to "always do something"; with both directions, clamping is the calmer
contract) and the carousel otherwise (wrapping both ways). Scroll wins on
the rare slide with both. Protocol: the monotonic `scrollSeq`/
`carouselSeq` counters stay, joined by `scrollDir`/`carouselDir` (+1/-1)
in `BlockLiveState`; an absent dir means forward, so old output pages
keep one-way semantics. The console is deliberately click-only (no
keyboard — the deck lives inside CardDetail's Escape/Ctrl+Enter-owning
modal; see §8.1). Planned article slides page on this same axis with no
new controls.

**Fullscreen media (`videoFull` 2026-07-25 / `imageFull` 2026-07-31):**
the console's ⛶ toggle enlarges the current slide's video — or, on
video-less image slides, its visible image (a gallery's active entry) —
to fill the `/present` viewport. Deliberately an in-page overlay
(`body.video-full` / `body.image-full` CSS classes in present.html), NOT
the browser Fullscreen API, so OBS captures the tab as-is. The flags live
in `BlockLiveState`; toggling never bumps `videoSeq` (enlarging must not
restart playback). One ⛶ per console row: video's when the slide has
video, image's otherwise. Slide navigation clears both overlays (state is
replaced wholesale).

---

## 15. Document Editor — `DocumentEditor` + format modules (`shared/documentFormats/`)

**Files:** `components/workspace/editor/` (`DocumentEditor`, `EditorToolbar`, `EditorPane`, `PreviewPane`, `DocumentOutline`, `EditorStatusBar`), `lib/editor/` (`codemirror` action, `DocumentSession`, `DocumentSource`, `languages`, `theme`, `fountain`, `documentPrefs`), `shared/documentFormats/` (registry + `markdown`, `fountain`, `plaintext` modules). Design: `plan/2026-09-06 document formats - markdown, fountain, manuscript.md`.

The workspace file viewer is a **CodeMirror 6 document editor** for plain-text formats that render into something else. `WorkspaceFileViewer` is only the overlay; it hands `DocumentEditor` a `DocumentSource` (`open` / `stat` / `save`) built from the workspace RPCs. **Nothing in the editor assumes workspace files.** The second source is a **text attachment** (`attachmentDocumentSource` → `Open/Stat/SaveCardAttachmentText`, overlay `AttachmentDocumentViewer`): `CardAttachments` opens anything `shared/attachmentText.ts` `isEditableTextAttachment` accepts (registry formats, common text extensions, `text/*`) in the editor instead of a preview — same guard, same autosave, no open-externally/reveal buttons because there is no folder. Editors are reached from where the work is defined — the card's Workspace Files block, the attachment list — and from the panel's tree; the clone is never what BRUV edits.

**Format registry (compile-time, no marketplace).** `documentFormatForPath(path)` picks a `DocumentFormat` by extension (`.md` → Markdown, `.fountain` → Fountain, anything else → plain text). A module is pure functions over the text — `render` (preview HTML), `styles` (preview + `@media print` CSS scoped under `.doc-preview`), `outline`, `entities`, `lint`, `wordCount`, `printable` — so it runs in Vitest and on mobile. The one desktop-only piece, the CodeMirror language/highlighting, lives in `lib/editor/languages.ts` keyed by format id; `shared/` never imports CodeMirror. **Fountain has one grammar**: `classifyFountain` feeds both the editor decorations (`lib/editor/fountain.ts`) and the renderer.

| `DocumentEditor` prop | Type | Notes |
|---|---|---|
| `source` | `DocumentSource` | Where the bytes come from; one `DocumentSession` per source |
| `onClose` | `() => void` | Called only after the draft is on disk or explicitly discarded |
| `onOpenExternal` / `onReveal` | `() => void` | Tier 1 fallbacks; shown in the toolbar and in the load-error state when provided |

Exported for the host: `requestClose()` (backdrop click) and `onKeydown(e)` (the host's `<div role="dialog">` forwards every keydown).

**Layouts** edit / split / preview, remembered **per format** in client-zone UI preferences (`document_layouts`, `document_outline` — `lib/editor/documentPrefs.svelte.ts`). The editor pane stays mounted in preview mode (hidden, not removed) so undo history and the cursor survive a flip. Outline click = `goToLine`; the entry under the cursor is highlighted.

**Autosave + divergence guard (`DocumentSession`).** Debounced autosave 1 s after the last edit; the save presents the stamp (`sha256`) it loaded, and the backend refuses a save when the file on disk changed meanwhile (`SaveWorkspaceFile` → `diverged: true`, nothing written). Policy, never silent clobbering: window focus stats the file — clean draft → reload quietly + an info toast; dirty draft → `showConfirm` reload-or-keep; a refused save → `showConfirm` overwrite-or-keep. **Keep pauses autosave** (status bar says so, with a Save button) so the prompt cannot re-fire per keystroke; the next explicit save asks again. Save state is ambient in the status bar (§9) — **no toast per save**; a failed autosave shows inline and retries on the next edit — **never on a timer loop** (a thrown save does not reschedule itself). `flush()` waits for an in-flight save, then saves whatever was typed since (it used to return false mid-autosave, so Escape asked "discard?" and lost that text). A failed focus-time reload shows inline (`syncError`, `document.reload_failed`) instead of rejecting. If the editor unmounts without `requestClose` (the card closed underneath, the source swapped) `session.close()` saves a pending draft, and a draft it couldn't save is reported with a toast.

**Keyboard.** All keys stay inside the editor (`stopPropagation` on the dialog — the board's `p`/`w`/`?` shortcuts and any card dialog beneath never see them, §8.1). Escape and Ctrl+Enter both **flush then close**; a draft that cannot be saved asks before being discarded. CodeMirror gets first go (`defaultPrevented` is respected): Escape closes its search panel first, Ctrl+S saves now, Ctrl+F finds. CodeMirror's own strings are localized through `EditorState.phrases` (`lib/editor/phrases.ts`, keys `document.cm.*`).

**Theme.** `lib/editor/theme.ts` builds the CodeMirror theme from design tokens only (`var(--…)`) — dark/light follow the app with no JS. Prose-first: app font, monospace only for code; Fountain switches the content to Courier and indents cues/dialogue the way the page reads.

**Print** (formats with `printable`): the toolbar button adds `body.printing-document` around `window.print()`; `style.css` hides everything but `.doc-preview` and un-fixes the overlay so pages can flow; the format's `@media print` rules (Fountain: Letter, 1.5in left margin, notes/sections hidden) apply on top.

**Adding a format:** a module in `shared/documentFormats/<id>/` registered in `documentFormats/index.ts` (+ `DocumentFormatId`), a `document.format_<id>` label, a language bundle in `lib/editor/languages.ts` if it highlights, and golden tests in `frontend/src/lib/` against the format's own spec examples.

**Mobile:** `mobile/src/components/DocumentSheet.svelte` — the same two sources over `repoRPC` (`mobile/src/lib/documentSource.ts`), rendered through the format's `render` + `styles` in body-size wrapped typography (the reason it exists: a text attachment used to open as raw Latin-1 monospace in a browser tab), a plain textarea to edit with the same 1 s autosave + stamp guard (a refused save asks overwrite-or-reload via `ConfirmDialog`). Deliberate asymmetry: no CodeMirror, outline, print or template generation on the phone.


---

## 16. @Mentions — `shared/mentions.ts`, `mentionable`, one picker per surface

**Markup.** A mention is plain Markdown with a `bruv:` link: `[Card title](bruv:card:<id>)` (projects: `bruv:project:<id>`). `shared/markdown.ts` renders it as `<a class="bruv-link" data-bruv="card:<id>">` with no `href`; each surface's click delegate navigates (desktop `main.ts`, mobile `lib/mentions.svelte.ts` `installMentionNavigation`). The MCP tool descriptions teach assistants the same markup so nobody pastes raw ids.

**Every text editor is mentionable (ruling 2026-09-20).** Typing `@` at the start of a field or after whitespace opens the picker; choosing an entry splices the mention over the `@…` and refocuses the field. The picker opens only on the keystroke that types the `@` (`isMentionKeystroke` — an `insertText`/composition input event whose data ends in `@`): dismissing it leaves the `@` in place, and backspacing or arrowing the caret back next to that `@` must NOT re-open it (ruling 2026-09-20). Deletions, pastes and the synthetic input event fired by `applyMentionToElement` never trigger. The trigger and splice live once in `shared/mentions.ts` (`detectMentionTrigger`, `spliceMention`, `applyMentionToElement`); the two hand-rolled copies that used to live in CardDetail and CardBlocks are gone (one had rotted into dead code). Wire a new editor with the `mentionable` action — `frontend/src/lib/mentions.svelte.ts` on desktop, `mobile/src/lib/mentions.svelte.ts` on mobile — placed **before** `use:inlineEdit` / `use:draftEdit` in the markup so its listeners register first. Wired today: description, text blocks, `EditableText` (every inline field incl. checklist/list items), checklist/list add-inputs, media and image captions, survey prompts/options/answers, comment composer and edit (desktop); description, text blocks, `EditableItemText`, comments, survey answers (mobile).

**One picker instance per surface.** Desktop: `MentionPicker` mounted once in `App.svelte`, driven by `mentionHost` (anchored under the field, z-index 200). Mobile: `MentionSheet` (a `BottomSheet` with search + recents) mounted once in `App.svelte`. The picker's Enter/Escape are consumed so the card dialog underneath never sees them.

**Blur is suspended while the picker is open.** The picker takes focus; that blur must not commit or unmount the field. `shared/mentions.ts` keeps a `WeakSet` of fields with a picker open (`setMentionPickerOpen` / `isMentionPickerOpenFor`); `inlineEdit`, `draftEdit`, and the hand-rolled description/text-block blur handlers all check it. Never add a new blur-commit path without that check.

**Labels follow the card.** The stored label freezes at insert time (`[Old Title](…)`) and nothing on disk rewrites it on rename — a rename would otherwise strand every mention. Instead links are refreshed where they render: `lib/mentionLabels.ts` (desktop) / `installMentionLabelRefresh` (mobile) watch the DOM, set each card link's text to the live title and its tooltip to the breadcrumb from a per-session cache, and a `card:updated` event invalidates that id. Stored text stays as written.

**Rendering.** Anything that shows user text renders it through `renderInline` / `renderMarkdown` so mentions are links: this pass moved desktop comment bodies and image captions onto the renderer. A new display of user text must not print the raw string. `.bruv-link` styling is in `frontend/src/style.css` and `mobile/src/app.css` (⌘ glyph, dashed underline).

---

## 17. Model choices — `ModelChoiceSelect`, `ChatModelChip`, `shared/modelRefs.ts`

Design: `plan/2026-09-25 multiple models and model routing.md` (BRUV card 1ccf828c).

**One value type.** Every setting that picks what serves an AI request stores a `ModelRef` string: `model:<id>`, `router:<id>`, `tier:<fast|balanced|powerful>` (router targets only) or `''` = inherit. Precedence at request time (backend `llmsvc.Select`): the chat's / agent's own choice → Settings → AI → Use for (task) → Default → first enabled model. `shared/modelRefs.ts` previews that precedence for labels (`effectiveRef`, `refLabel`) — it doesn't know eligibility, so the authority on what answered is the message's `route`.

**One picker per surface.** Desktop: `ModelChoiceSelect` (a native `<select>`: models grouped under providers in provider order, then tiers if `tiers`, then routers if `routers`; `inheritLabel` adds the `''` option; `variant="chip"` for chat headers). Disabled models are hidden unless they are the current value; a dangling ref shows as "Missing" rather than silently changing. Mobile: the chat sheet's `ChatModelChip` (native select → OS picker sheet). Model / provider / router configuration is desktop-only.

**Pickers never see keys.** Chat chips and the Agent tab read `GetLLMRegistry` (routing + provider id/label/kind) through `llmRegistry` / `loadLLMRegistry` (`frontend/src/lib/llmRegistry.ts`, `mobile/src/lib/llmRegistry.ts` over `shared/llmRouting.svelte.ts`). Only Settings loads full accounts. Settings reloads the registry after Save.

**Deleting clears references.** Removing a model or router goes through `removeModel` / `removeRouter`, which clear every task / default / rule / fallback pointing at it (rule targets and fallbacks fall back to `tier:balanced`). Deleting a model, provider or router confirms; removing a rule row inside a router is an edit (no confirm, §12.5).

**Explain what answered.** Assistant replies carry `route` (`RouteDecision`): desktop shows `· <model>` after the time with `describeDecision` as the tooltip; mobile makes the model name a button that toggles the same text under the message (no hover on touch). All explanation text is localized under `llm_routing.*` in both surfaces — the backend sends structured fields (`source`, `via`, `rule_index`, `band`, `score`, `skipped`), never prose.

**Grip rows.** Provider rows and router rule rows are sanctioned grip rows (§12.5), both driven by `RowReorder` (`frontend/src/lib/rowReorder.svelte.ts`) — use it for any new grip-reorder list instead of hand-writing the drag handlers.

---

## 18. Mobile reconnect — everything that loads, reloads (`lib/connectivity.svelte.ts`)

A network-level failure raises the full-screen offline overlay; on recovery `onReconnect` handlers run in place (no page reload). **Rule (2026-09-26): anything on screen that loads from the server registers an `onReconnect` handler** — pages, and components that load their own data (comments, notifications panel, chat sheet, chat model chip) — because the SSE events published during the outage are lost. Handlers:

- refetch what failed, and refresh what's shown unless the user is mid-edit (`editScope.hasActive()` / a save in flight) — an edit session always wins over the refresh;
- flush queued offline saves before refreshing (CardPage: `flushPendingSaves().then(refreshCardQuietly)`);
- clear their own error on a successful reload;
- guard overlapping loads with a sequence number when a load can hang on a dead socket (RepoPickerPage) — only the newest may write state.

App-level: `App.svelte` restarts the SSE stream (`restartEvents`) and heals `repoMeta`. Decorative labels must not wait on the network: the Browse header shows the cached vault name (`readActiveRepoName`) and never sticks on "Loading…".

---

## 19. Settings forms load and save per section (`lib/settingsSections.svelte.ts`)

A settings form backed by several Get/Set RPC pairs declares each pair as a section in `SettingsSections` (load + save). Sections load independently; a tab's fields render inside `SettingsSectionGate` only once its sections are ready — a failed section shows its error with **Try again** and the note that it won't be saved. **Save writes only sections that loaded** (a form must never persist the placeholder defaults it started with over settings it couldn't read — the pre-2026-09-26 `Promise.all` + `catch { use defaults }` did exactly that). Save is disabled while anything is loading; if some section's save fails the dialog stays open with the user's edits and a toast names the failed sections. Helpers that persist mid-edit (model Test / Find models) check the section is ready first.
---

## 20. Running-agent indicator — `.agent-running-dot` + global `agent-neon`

Anything that shows an agent is **running right now** uses the shared shimmer: the `--agent-running-gradient` / `--agent-running-glow*` tokens animated by the global `@keyframes agent-neon` in `style.css`. Components reference `agent-neon` and **never redefine it** (four copies existed until 2026-09-29). For a small inline marker use the global `.agent-running-dot` class.

**Source of truth:** `board.runningAgentIds` (`lib/store.svelte.ts`) — seeded from `GetAllAgents().is_running` on every board load (so a run that began before the app opened still shows) and kept live by `agent:started` / `agent:completed` / `agent:failed`. Read it with a `$derived`; don't poll.

**Where it appears:** the board tile (`CardItem`), the agents page and dashboard rows, the card detail tab bar (the Agent tab's dot shimmers instead of the static "has an agent" dot, so it's visible from every tab, with the localized `agent.running` tooltip), and a live "Running..." row at the top of the Runs tab showing `agent.running_since`. Reduced motion is handled by the global `prefers-reduced-motion` rule.
---

## 21. Agent permissions — `AgentToolPicker` + `lib/agentToolGroups.ts`

**Contract (2026-09-29): the Agent tab shows a tick against every permission the agent actually has, whatever is stored.** An agent can use only the tools it was granted; an empty list grants none — the picker then shows "No tools granted: the agent can only reply in text."

- **Rows come from the backend**, never a hand-kept list: `DescribeAgent(cardId).options.tools` is every tool an agent can be granted (agent-only built-ins plus BRUV's native board tools; the agent-management tools `configure_card_agent` / `run_card_agent` are never grantable). `buildToolGroups` groups them — Web, This card & alerts, Read the board, Create & edit cards, Filing, Brands/streams/projects — and anything new lands in **Other tools** rather than disappearing.
- **MCP servers** get one group each, listing every tool with its own checkbox. A partly granted server shows its header **indeterminate**, not unticked. A server that isn't ready still lists its granted tools, marked *Not available right now — still granted*.
- **Nothing is hidden:** a granted id the backend no longer offers (a retired tool, a removed server) appears ticked under **Granted but not available**, so the user can see and revoke it.
- Group headers are tri-state; ticking a partly ticked header grants the rest.

| Prop | Type | Notes |
|---|---|---|
| `allowedTools` | `string[]` (bindable) | The agent's `allowed_tools`. |
| `options` | `AgentToolOption[]` | From `DescribeAgent`. |
| `mcpServers` | `MCPServerView[]` | From `ListMCPServers`. |
| `onchange` | `() => void` | Fired after a grant or revoke (the tab marks itself dirty). |

Labels: built-ins keep `agent.tool_<id>` (+ `_desc`); native tools use `agent.tool.<id>`, falling back to the id for an unknown tool. Desktop only — mobile has no agent editor.
---

## 22. Slide media fields are lists — `MediaRefList` + `lib/slides/mediaValue.ts`

A slide media field's literal value is a list: image fields hold every gallery URL / `attachment:` ref joined with `'\n'` (the renderer shows a carousel), video fields one. `BoundField` renders media fields through `MediaRefList`: one row per item — an attachment ref is a chip (name via `refDisplayName`), a URL is its own single-line input — each with its own remove ✕; *Add URL* adds a row (image fields), *Pick an attachment* appends a ref (image) or replaces the value (video), and pasting several lines creates one row per line. Rows have stable ids; the value is re-joined (blank rows dropped) on every change. Never edit a joined media value in one `<input>` — inputs strip line breaks, which merged gallery URLs, and a single chip's ✕ used to wipe every image.

| Prop | Type | Notes |
|---|---|---|
| `value` | `string` | Newline-joined items. |
| `multiple` | `boolean` | `true` for image fields (gallery), `false` for video. |
| `attachmentOptions` | `{ ref, name, fromLinked }[]` | Media attachments of the host + linked card. |
| `refDisplayName` | `(ref: string) => string` | Chip label for an `attachment:` ref. |
| `onChange` | `(value: string) => void` | The re-joined value. |
