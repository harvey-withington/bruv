<script lang="ts">
  // The Save-as name prompt, shown under the editor toolbar: the name
  // field starts as "name (copy).ext", selected; Enter saves, Escape
  // cancels (and stays inside the bar — the editor would otherwise close).
  import { t } from '../../../lib/i18n.svelte'
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

  function onKeydown(e: KeyboardEvent) {
    if (e.isComposing) return
    if (e.key === 'Enter') {
      e.preventDefault()
      e.stopPropagation()
      if (name.trim()) onSave(name)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      onCancel()
    }
  }
</script>

<div class="save-as" role="group" aria-label={t('document.save_as')}>
  <span class="label">{t('document.save_as_label')}</span>
  <input class="name" bind:value={name} use:focusSelect onkeydown={onKeydown} aria-label={t('document.save_as_label')} disabled={busy} />
  <button class="btn primary" onclick={() => onSave(name)} disabled={busy || !name.trim()}>
    {busy ? t('common.working') : t('document.save_as_confirm')}
  </button>
  <button class="btn subtle" onclick={onCancel} disabled={busy}>{t('common.cancel')}</button>
  <span class="hint">{t('document.save_as_hint')}</span>
</div>

<style>
  .save-as {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.45rem;
    padding: 0.45rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-surface);
    font-size: 0.8rem;
  }
  .label { color: var(--text-muted); }
  .name {
    flex: 1;
    min-width: 12rem;
    background: var(--bg-elevated);
    border: 1px solid var(--accent);
    border-radius: 5px;
    padding: 0.3rem 0.5rem;
    color: var(--text-primary);
    font: inherit;
    outline: none;
  }
  .hint { flex-basis: 100%; color: var(--text-muted); font-size: 0.72rem; }
</style>
