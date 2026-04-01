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
| `markdown` | `boolean` | `false` | Render value as markdown when not editing |
| `inputClass` | `string` | `''` | Extra CSS class for the input/textarea |
| `displayClass` | `string` | `''` | Extra CSS class for the display element |
| `onSave` | `(value: string) => void` | — | Called when the user commits a change |

**Keyboard behaviour:**
- **Click** or **Enter** (when focused) → enters edit mode, selects all text.
- **Enter** → saves (in single-line mode).
- **Escape** → cancels edit, reverts to original value.
- **Blur** → saves.

**Example:**

```svelte
<EditableText
  value={card.title}
  placeholder="Untitled"
  onSave={(v) => updateTitle(v)}
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

## 5. General Accessibility Guidelines

1. **All interactive elements must be keyboard-reachable.** Use `tabindex="0"` on non-button elements that act as buttons.
2. **Focus-visible must match hover state.** If a button turns red on hover, it must also turn red on `:focus-visible`.
3. **Escape always cancels** an in-progress edit without saving.
4. **Enter saves** in single-line edits; **Ctrl+Enter** saves in multiline edits.
5. **Use semantic HTML** — prefer `<button>` for actions, `<input>` for editable text.
6. **Never remove elements from the DOM** to hide them — use `color: transparent` or `opacity: 0` so they remain accessible to screen readers and keyboard navigation.
