<script lang="ts">
  // The Save-as name prompt under the document sheet's header: starts as
  // "name (copy).ext" with the stem selected; Done / the keyboard's Go
  // saves. Escape is the sheet's (it cancels this bar first).
  import { t } from '../lib/i18n.svelte'
  import { copyName } from '@shared/documentSource'

  let { original, busy, onSave, onCancel }: {
    original: string
    busy: boolean
    onSave: (name: string) => void
    onCancel: () => void
  } = $props()

  // Captured once — the bar is remounted per open.
  /* svelte-ignore state_referenced_locally */
  let name = $state(copyName(original))

  function focusSelect(node: HTMLInputElement) {
    node.focus()
    const dot = node.value.lastIndexOf('.')
    node.setSelectionRange(0, dot > 0 ? dot : node.value.length)
  }
</script>

<form class="save-as" onsubmit={(e) => { e.preventDefault(); if (name.trim()) onSave(name) }}>
  <label class="label" for="save-as-name">{t('document.save_as_label')}</label>
  <div class="row">
    <input id="save-as-name" class="name" bind:value={name} use:focusSelect enterkeyhint="go" disabled={busy} />
    <button type="submit" class="act primary" disabled={busy || !name.trim()}>{busy ? t('common.working') : t('document.save_as_confirm')}</button>
    <button type="button" class="act" onclick={onCancel} disabled={busy}>{t('common.cancel')}</button>
  </div>
  <span class="hint">{t('document.save_as_hint')}</span>
</form>

<style>
  .save-as {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 0.6rem 0.9rem;
    border-bottom: 1px solid var(--border);
    background: var(--bg-elev-1);
  }
  .label { font-size: 0.8rem; color: var(--text-muted); }
  .row { display: flex; gap: 0.4rem; }
  .name {
    flex: 1;
    min-width: 0;
    min-height: 40px;
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: 8px;
    padding: 0 0.6rem;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    outline: none;
  }
  .act {
    min-height: 40px;
    padding: 0 0.75rem;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .act.primary { border-color: var(--accent); color: var(--accent); font-weight: 600; }
  .act:disabled { opacity: 0.6; cursor: default; }
  .hint { font-size: 0.75rem; color: var(--text-faint); }
</style>
