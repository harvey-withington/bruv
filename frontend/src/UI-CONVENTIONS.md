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
- **Multiline fields: virtual Enter inserts a newline** (platform-native — Shift+Enter needs two fingers on a touch keyboard). Commit = tap-away (blur) or the explicit ✓ Done button on the active editor. Chat: Enter = newline, the Send button sends. Hardware chords still work everywhere: Ctrl+Enter commits (+closes), Escape cancels. Implemented via the `enterInsertsNewline` option on `inlineEdit`/`draftEdit` — desktop never sets it.
- **Back = Escape**: while an edit is active, Back (app ← button, gesture, hardware) cancels the edit and the page stays open; with nothing active it navigates. Composers (chat, quick capture, comments) preserve their draft on a Back-cancel — an accidental back-swipe must never eat a typed message. A cancel (or commit) never moves the page's scroll position — the mobile router sets `history.scrollRestoration = 'manual'` so the undone traversal can't re-apply a stale scroll offset.
- **Checklist/list item rows: Enter (the keyboard's ✓ tick) JUST commits — no next-row advance** (ruling, 2026-07-10). Adding rows is the + button's job (it spawns a blank auto-edit row); one + tap per item is the accepted trade-off. Desktop's serial add-input keeps its re-arm behaviour — this is a mobile-surface rule only.
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

**Container side — `EditScope`.** Each closable container (card dialog, modal dialog, mobile sheet) creates a scope, sets `requestClose`, and shares it via `setContext(EDIT_SCOPE_KEY, scope)`. Nested dialogs create their own scope — context shadowing routes fields to the nearest container. The container's window keydown asks `scope.hasActive()` before closing on Escape and calls `scope.commitAll()` for the Ctrl+Enter chord (`scope.handleWindowKeydown` is a ready-made helper). The scope also feeds "don't clobber my edit" guards (e.g. CardDetail skips silent card reloads while the scope has active edits).

**Do not** write inline `onkeydown` + `onblur` handlers for simple commit/cancel inputs — use this action.

### 8.1 Layered dialogs — shield BOTH window keys (recurring data-loss bug)

Containers like CardDetail own **two** window-level keys: Escape (close when nothing edits) and **Ctrl+Enter (commit-all + close)**. Any dialog/sheet/picker layered above such a container MUST intercept **both** — every Escape-only shield has later resurfaced as a Ctrl+Enter bug that closed the card underneath and discarded the layer's edits (SlideEditorDialog hit this twice: Escape 2026-07-18, Ctrl+Enter 2026-07-27).

In the layer, route the keys per the §8 table *scoped to the layer itself*: Escape cancels/closes the topmost thing (open dropdown first, then the layer), Ctrl+Enter commits **the layer's surface**, not the card's.

Two working shield patterns — pick by DOM relationship to the parent's listener:

1. **Child-side capture shield** — a capture-phase window listener (`window.addEventListener('keydown', h, true)`) registered while the layer is open; `preventDefault()` + `stopPropagation()` stops the parent's bubble-phase `<svelte:window>` handler. Reference: `SlideEditorDialog.handleCaptureKeydown`, mobile `ChatSheet`.
2. **Parent-side stand-down guard** — required when the layer is a portaled **sibling** that itself listens on `<svelte:window>`: `stopPropagation()` cannot stop sibling listeners on the same node (only `stopImmediatePropagation`, unavailable from `<svelte:window>`). The parent checks the layer's visibility and returns early. Reference: CardDetail's `optionsEditorState.visible` / `showCreateTypeDialog` / `showPromoteDialog` guards.

These are stopgaps: the overlay-stack refactor in `plan/TODO.md` replaces all of them with one ordered stack. Until it lands, every new layered dialog adds one of the two shields — for both keys — or it ships this bug again.

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

Dropdown menus share the global `.dropdown-menu` / `.dropdown-menu-item` classes (+ `.dropdown-menu--grid` for grid layouts) in `style.css` — same shared-utility pattern as `.action-reveal`. Discrete dropdowns also close on **Escape** (consumed) per §8/§12.5, and every `:hover` style needs its `:focus-visible` twin (§12.2). Reference implementations: `CardShareMenu.svelte`, `BlockPicker.svelte`.

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

**Sanctioned exception (ruling, 2026-07-10):** rows whose *body is an edit surface* — clicking the row starts editing or another primary action — keep a `GripVertical` as their only drag surface, because the click has to mean "edit": BlockItem, EditableChecklist/EditableList item rows, SurveyBlock questions, OptionsEditorDialog/TemplateEditor/TemplateEditorDialog option & param rows, LLMAccountsManager account rows (click expands the inline editor), MCPServersDialog arg rows (text inputs), and mobile Checklist/ListBlock (where the grip also avoids the touch-scroll conflict). Do not "fix" grip-only dragging on such rows, and do not add grips anywhere else.

**Delete vs add-cancel (ruling, 2026-07-10):** clicking a **delete/clear button always confirms** via ConfirmDialog — even for empty containers and zero-usage tags. Cancelling an add-flow (Escape on an untouched placeholder, backing out of an add input) **never prompts**.

The boundary (Harvey, 2026-07-10): confirmation applies to deleting an **object** (card, category, brand/stream/project, tag, template, agent, attachment, comment, notification list). Removing a **row inside an editing surface** — a checklist/list item, media item, tag chip on a card, select option, survey question, MCP arg, template param, a single notification — is an *edit to the containing object*, not a delete, and stays promptless.

---

## 12.7 Action-Button Labels

**Short verb labels; tooltips carry the explanation (ruling, 2026-07-17).** Action buttons are labelled with the bare verb — "Delete", "Share", "Promote" — not verb + object ("Delete Card"). The surrounding context already names the object; the `title` tooltip holds the longer explanatory text (e.g. the card footer's delete button: `common.delete` label + `tooltip.delete_card` = "Delete this card permanently").

Bare-verb labels use the shared keys `common.add` / `common.delete` / `common.remove` / `common.save` (both surfaces) — don't mint per-context keys for a value that is just the verb. A per-context key is only warranted when the label genuinely differs (e.g. "Delete All").

---

## 13. SidePanel, Workspace Panel & WorkspaceFileTree

**`SidePanel.svelte`** is the single right-hand panel host: one resizable, slide-animated container with a VS Code-style bottom tab bar, rendered as a sibling of `<Board/>` in App's `.board-row`. It owns ALL geometry (width persistence, drag-to-resize, slide in/out via width-not-transform animation — transforms break WebView2 scroll containers). Content components fill 100% and own zero geometry: `ChatSection` runs in `hosted` mode (its own shell/resize/animation disabled; card chat keeps `hosted=false`), `WorkspacePanel` is geometry-less by construction. Both tab panes stay mounted — inactive ones hide via CSS (`.sp-tab-pane.pane-hidden`) so chat drafts/scroll survive tab flips. TopBar buttons open-and-focus their tab, or close the panel when their tab is already frontmost; keyboard `p` = chat tab, `w` = workspace tab.

Future-layout note: SidePanel is the deliberate seam for a horizontal split or a generic drag-drop panel scheme — consumers pass `tabs` + a content snippet keyed by tab id; only SidePanel internals change.

**Panel-header convention** (Harvey, 2026-07-05): every panel header title is **icon + Proper Case** — never all-caps/`text-transform: uppercase`. Reference style: `0.82rem`, weight 600, `var(--text-strong)`, icon size 15, gap `0.4rem` (see WorkspacePanel's `.title` / ChatSection's `.chat-title`). Tab labels match their pane's header title verbatim.

The Workspace panel content (`components/workspace/WorkspacePanel.svelte`) renders inside a SidePanel tab. Sub-dialogs (attach, template editor, file viewer) are self-contained overlay components in `components/workspace/`.

**`WorkspaceFileTree.svelte`** is the reusable recursive collapsible tree. It is **lazy**: one instance renders one directory level, and mounting it is what loads that directory.

| Prop | Type | Notes |
|---|---|---|
| `cache` | `WorkspaceDirCache` | Per-directory cache from `lib/workspaceTree.svelte.ts` (`createWorkspaceDirCache(loader)`), created ONCE by the root consumer |
| `dir` | `string` | Directory this instance renders the children of (`''` = root) |
| `onOpenFile` | `(path: string) => void` | File row click |
| `depth` | `number` | Indentation level (self-incremented on recursion) |
| `collapsed` | `Record<string, boolean>` | Shared expand state owned by the root consumer — what makes Expand/Collapse All and accordion mode global. **A folder is expanded only when `collapsed[path] === false`; absent = collapsed**, so the tree opens closed |
| `mode` | `'single' \| 'multi'` | `single` = accordion (expanding collapses siblings) |

**Never load the whole tree.** Rendering from one whole-workspace index made opening the panel cost everything on disk — one `node_modules`, a nested `.git`, or a folder of 80k photos anywhere under the root choked it. A server-side blocklist of heavy directory names was rejected (Harvey, 2026-08-02): *"a band-aid, not a solution — this issue would apply to a .git folder or many others neither you nor I can predict. The tree should load collapsed and load incrementally."* So the tree opens **collapsed**, and expanding a folder calls **`ListWorkspaceDir(brand, stream, project, rel)`** for that one folder. A directory nobody opens is never read.

`createWorkspaceDirCache(loader)` is the store: keyed by directory path (`''` = root), each entry a `WorkspaceDirState` union — `{status:'loading'}` / `{status:'ready', children, isTemplateRoot}` / `{status:'error', error}`. `ensure(dir)` fetches on a miss and is a no-op on ANY cached entry, so collapse + re-expand never refetches and a failed folder keeps its error until retried; `reload(dir)` is the retry; `clear()` drops everything and discards in-flight responses (generation counter) so a Refresh can't be overwritten by a stale listing. Sibling order is the server's (path-ascending, files and folders interleaved — *not* folders-first).

**Three distinct states per level, never conflated**: `common.loading` row while in flight, a `workspace.dir_failed` row with a `common.retry` button on failure, and *nothing at all* for a genuinely empty directory. A folder that failed to list must never render as empty.

**Expand All is bounded.** "Expand everything" would re-walk the whole workspace — the exact cost lazy loading exists to remove. The button expands only directories already in the cache and fetches nothing, and is labelled `workspace.expand_loaded` ("Expand loaded folders") rather than `sidebar.expandAll` so the label doesn't promise the tree. Collapse All stays global and is just `treeCollapsed = {}` (absent = collapsed).

**Folder-Template roots** get the cyan `LayoutTemplate` icon when *known*: lazily, a directory is only classifiable once its own children are loaded, so the rule is "a loaded directory whose children include a `.ft` folder". The icon appears when the folder is opened; nothing blocks on it.

`WorkspacePanel` owns the cache and calls `resetTree()` (clear + drop expand state) on Refresh and on project change; mounted levels re-fetch themselves because each level's `$effect` reads its cache entry and reloads when it goes missing — so a refresh costs what's on screen, not the tree.

It self-imports for recursion (never `<svelte:self>` — deprecated). Keyboard: rows are real `<button>`s, so Tab/Enter work for free.

**`WorkspaceLocalCopy.svelte`** — this device's working copy, rendered inside the Workspace panel **only on a remote connection** (`isLocalActive()` is false). On a remote connection the workspace's files are on the server, so Tier 1 actions have nothing local to act on until the device clones a copy. The component owns that whole lifecycle: report → confirm → the host publishes (`Workspace.git_serve`: `initializing` → `ready`/`error`, vault-side so every client sees it) → this device clones (shell-side status, polled ONLY while a clone runs) → pull/push/forget.

| Prop | Type | Notes |
|---|---|---|
| `ws` | `Workspace` | Carries `git_serve` — the host-side half of the lifecycle |
| `brandSlug` / `streamSlug` / `projectSlug` | `string` | Project coordinates for the publish RPCs |
| `serverName` | `string` | `activeConnectionLabel()` — every message names the machine |
| `onCheckoutChange` | `(info: WorkspaceCheckoutInfo \| null) => void` | Lets the panel point Tier 1 actions at the clone |

**Two machines means saying which one.** Every string in this section interpolates `{server}` — "Preparing the workspace on RIPPED", "git isn't installed on RIPPED". The bug this feature fixes was an error that read as "your folder is missing" when the folder was on the user's own disk and the *server* couldn't see it. Applies to the attach error too, which now names the machine it searched.

**Tier 1 actions follow the files, and hide when there are none.** `WorkspacePanel` derives `deviceRoot` — the origin path when the vault is served from this machine, otherwise the clone's path, `undefined` when neither. Open-folder and the launch command are hidden rather than offered-and-failing.

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
