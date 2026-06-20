<script lang="ts">
  // Crystallise the current card's layout into a reusable card type. Pick
  // a name (+ optional colour/icon) and which of the card's blocks become
  // the type's template; on create the backend strips the values, makes the
  // type, and switches this card to it. Mirrors CardTypeEditor's chrome.
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { showToast } from '../lib/toast.svelte'
  import { focusTrap, portal } from '../lib/actions'
  import { draggable } from '../lib/draggable'
  import { X, Smile } from 'lucide-svelte'
  import DynamicIcon from './DynamicIcon.svelte'
  import IconPicker from './IconPicker.svelte'
  import type { Block } from '@shared/types'

  let { blocks, onCreate, onClose }: {
    blocks: Block[]
    /** Performs the create. Throws on failure (the dialog shows a toast and
     *  stays open); resolves on success (the dialog closes). */
    onCreate: (name: string, icon: string, color: string, blockIDs: string[]) => Promise<void>
    onClose: () => void
  } = $props()

  const TYPE_PALETTE = [
    '#6366f1', '#22c55e', '#f59e0b', '#ec4899', '#06b6d4',
    '#ef4444', '#8b5cf6', '#f97316', '#10b981', '#3b82f6',
    '#e11d48', '#84cc16',
  ]

  let name = $state('')
  let color = $state(TYPE_PALETTE[0])
  let icon = $state('')
  let saving = $state(false)
  let showIconPicker = $state(false)
  // Default: every block is part of the template. Captured once on open —
  // the dialog is remounted per-open, so initial-value capture is intended.
  /* svelte-ignore state_referenced_locally */
  let selected = $state(new Set(blocks.map(b => b.id)))

  function blockLabel(b: Block): string {
    return b.label || b.key || b.type
  }

  function toggle(id: string) {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selected = next
  }

  async function create() {
    if (!name.trim()) { showToast(t('create_type_from_card.name_required'), 'error'); return }
    saving = true
    try {
      await onCreate(name.trim(), icon, color, [...selected])
      onClose()
    } catch (e) {
      const detail = e instanceof Error && e.message ? `: ${e.message}` : ''
      showToast(t('create_type_from_card.err') + detail, 'error')
    } finally {
      saving = false
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (showIconPicker) return
      onClose()
    }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div
    class="dialog"
    role="dialog"
    tabindex="-1"
    aria-label={t('create_type_from_card.title')}
    use:draggable={{ handle: '.dialog-header' }}
    use:focusTrap
  >
    <div class="dialog-header">
      <h2>{t('create_type_from_card.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <div class="field-row">
        <span class="field-label">{t('create_type_from_card.name')}</span>
        <!-- svelte-ignore a11y_autofocus -->
        <input
          class="field-input"
          bind:value={name}
          autofocus
          placeholder={t('create_type_from_card.name_placeholder')}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); create() } }}
        />
      </div>

      <div class="field-row">
        <span class="field-label">{t('card_type_editor.color')}</span>
        <div class="color-palette">
          {#each TYPE_PALETTE as c}
            <button class="color-swatch" class:active={color === c} style:background={c} onclick={() => color = c} aria-label={c}></button>
          {/each}
        </div>
        <div class="color-preview">
          <span class="type-badge-preview" style:background={color}>
            {#if icon}<DynamicIcon name={icon} size={12} className="badge-icon" />{/if}
            {name || t('create_type_from_card.name_placeholder')}
          </span>
        </div>
      </div>

      <div class="field-row">
        <span class="field-label">{t('icon.pick')}</span>
        <div class="icon-picker-row">
          <button class="icon-picker-btn" onclick={() => showIconPicker = true}>
            {#if icon}
              <DynamicIcon name={icon} size={16} />
              <span class="icon-name">{icon}</span>
            {:else}
              <Smile size={16} />
              <span class="icon-name muted">{t('icon.none')}</span>
            {/if}
          </button>
        </div>
      </div>

      <div class="field-row">
        <span class="field-label">{t('create_type_from_card.blocks')}</span>
        {#if blocks.length === 0}
          <p class="empty-note">{t('create_type_from_card.no_blocks')}</p>
        {:else}
          <div class="block-list">
            {#each blocks as b (b.id)}
              <label class="block-row">
                <input type="checkbox" checked={selected.has(b.id)} onchange={() => toggle(b.id)} />
                <span class="block-name">{blockLabel(b)}</span>
                <span class="block-type">{b.type}</span>
              </label>
            {/each}
          </div>
          <span class="hint">{t('create_type_from_card.blocks_hint')}</span>
        {/if}
      </div>
    </div>

    <div class="dialog-footer">
      <button class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn-primary" onclick={create} disabled={saving || !name.trim()}>
        {saving ? t('common.saving') : t('create_type_from_card.create')}
      </button>
    </div>
  </div>

  {#if showIconPicker}
    <IconPicker
      value={icon}
      onSelect={(i) => { icon = i; showIconPicker = false }}
      onClose={() => showIconPicker = false}
    />
  {/if}
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 440px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    overflow: visible;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .dialog-header h2 { font-size: 1.1rem; font-weight: 600; margin: 0; }
  .close-btn { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 0.25rem; line-height: 1; border-radius: 4px; }
  .close-btn:hover { color: var(--text-primary); }
  .dialog-body { padding: 1.25rem; overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: 0.85rem; min-height: 0; }
  .dialog-footer { padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted); display: flex; justify-content: flex-end; gap: 0.5rem; }

  .field-row { display: flex; flex-direction: column; gap: 0.35rem; }
  .field-label { font-size: 0.85rem; font-weight: 500; color: var(--text-muted); }
  .field-input {
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.45rem 0.6rem;
    color: var(--text-primary);
    font-size: 0.85rem;
    width: 100%;
    box-sizing: border-box;
    outline: none;
    font-family: inherit;
  }
  .field-input:focus { border-color: var(--accent); }

  .color-palette { display: flex; flex-wrap: wrap; gap: 6px; }
  .color-swatch {
    width: 22px; height: 22px;
    border-radius: 50%;
    border: 2px solid transparent;
    cursor: pointer;
    transition: transform 0.1s;
  }
  .color-swatch:hover { transform: scale(1.15); }
  .color-swatch.active { border-color: var(--text-primary); }

  .color-preview { display: flex; align-items: center; gap: 8px; margin-top: 4px; }
  .type-badge-preview {
    font-size: 0.7rem;
    font-weight: 600;
    color: #fff;
    padding: 2px 10px;
    border-radius: 999px;
    display: inline-block;
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .icon-picker-row { display: flex; align-items: center; }
  .icon-picker-btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    font-size: 0.85rem;
    cursor: pointer;
  }
  .icon-picker-btn:hover { border-color: var(--accent); }
  .icon-name { font-size: 0.8rem; }
  .icon-name.muted { color: var(--text-muted); }
  :global(.badge-icon) { margin-right: 2px; }

  .block-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 220px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 4px;
    background: var(--bg-elevated);
  }
  .block-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.35rem 0.5rem;
    border-radius: 5px;
    cursor: pointer;
  }
  .block-row:hover { background: var(--bg-hover); }
  .block-row input { cursor: pointer; }
  .block-name {
    flex: 1;
    min-width: 0;
    font-size: 0.85rem;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .block-type {
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    background: var(--bg);
    padding: 1px 6px;
    border-radius: 3px;
    flex-shrink: 0;
  }
  .empty-note { margin: 0; font-size: 0.8rem; color: var(--text-muted); font-style: italic; }
  .hint { font-size: 0.7rem; color: var(--text-muted); }

  .btn-primary {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
  }
  .btn-primary:hover { opacity: 0.9; }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-secondary {
    background: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    cursor: pointer;
  }
  .btn-secondary:hover { background: var(--bg-hover); }
</style>
