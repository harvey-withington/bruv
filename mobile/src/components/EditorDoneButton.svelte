<script lang="ts">
  // ✓ Done — the explicit commit affordance for mobile multiline
  // editors (UI-CONVENTIONS §8 mobile variant). With Enter inserting a
  // newline, tap-away (blur) is the implicit commit; this button is the
  // guaranteed safe tap target that does the same thing visibly.
  //
  // pointerdown is prevented so tapping it never blurs the field first —
  // onDone owns the whole commit (and dismisses the keyboard itself),
  // instead of racing a blur-commit. The click handler is the fallback
  // for keyboard/AT activation, which produces no pointerdown.

  import { Check } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  let { onDone }: { onDone: () => void } = $props()

  let handledByPointer = false

  function handlePointerDown(e: PointerEvent) {
    e.preventDefault() // keep focus in the field — no blur-commit race
    handledByPointer = true
    onDone()
  }

  function handleClick() {
    if (handledByPointer) {
      handledByPointer = false
      return
    }
    onDone()
  }
</script>

<button
  type="button"
  class="editor-done"
  onpointerdown={handlePointerDown}
  onclick={handleClick}
  aria-label={t('editor.done')}
  title={t('editor.done')}
>
  <Check size={16} />
</button>

<style>
  .editor-done {
    background: var(--accent);
    border: none;
    color: var(--bg);
    border-radius: 8px;
    min-width: 36px;
    min-height: 36px;
    padding: 0.35rem;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    touch-action: manipulation;
  }
  .editor-done:hover,
  .editor-done:focus-visible {
    filter: brightness(1.08);
    outline: none;
  }
</style>
