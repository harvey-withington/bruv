<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { X } from 'lucide-svelte'
  import { focusOnMount } from '../lib/actions'

  let {
    initial = '#ffffff',
    onApply,
    onClose,
  }: {
    initial?: string
    onApply: (color: string) => void
    onClose: () => void
  } = $props()

  // Curated palette — rows are hue families, columns are shades.
  // Picked to feel useful for branding without being overwhelming.
  const PALETTE: string[][] = [
    ['#000000', '#1f2937', '#374151', '#6b7280', '#9ca3af', '#d1d5db', '#f3f4f6', '#ffffff'],
    ['#7f1d1d', '#b91c1c', '#dc2626', '#ef4444', '#f87171', '#fca5a5', '#fecaca', '#fee2e2'],
    ['#7c2d12', '#c2410c', '#ea580c', '#f97316', '#fb923c', '#fdba74', '#fed7aa', '#ffedd5'],
    ['#78350f', '#b45309', '#d97706', '#f59e0b', '#fbbf24', '#fcd34d', '#fde68a', '#fef3c7'],
    ['#365314', '#4d7c0f', '#65a30d', '#84cc16', '#a3e635', '#bef264', '#d9f99d', '#ecfccb'],
    ['#14532d', '#15803d', '#16a34a', '#22c55e', '#4ade80', '#86efac', '#bbf7d0', '#dcfce7'],
    ['#134e4a', '#0f766e', '#0d9488', '#14b8a6', '#2dd4bf', '#5eead4', '#99f6e4', '#ccfbf1'],
    ['#164e63', '#0e7490', '#0891b2', '#06b6d4', '#22d3ee', '#67e8f9', '#a5f3fc', '#cffafe'],
    ['#1e3a8a', '#1d4ed8', '#2563eb', '#3b82f6', '#60a5fa', '#93c5fd', '#bfdbfe', '#dbeafe'],
    ['#312e81', '#4338ca', '#4f46e5', '#6366f1', '#818cf8', '#a5b4fc', '#c7d2fe', '#e0e7ff'],
    ['#581c87', '#7e22ce', '#9333ea', '#a855f7', '#c084fc', '#d8b4fe', '#e9d5ff', '#f3e8ff'],
    ['#831843', '#be185d', '#db2777', '#ec4899', '#f472b6', '#f9a8d4', '#fbcfe8', '#fce7f3'],
  ]

  // svelte-ignore state_referenced_locally
  let current = $state(normalizeHex(initial) ?? '#ffffff')
  // svelte-ignore state_referenced_locally
  let hexInput = $state(current)

  // Recent colours: persisted in localStorage, capped at MAX_RECENT, MRU order.
  // Updated whenever a colour is committed via Apply or double-click.
  const RECENTS_KEY = 'bruv:colorPickerRecents'
  const MAX_RECENT = 10
  let recents = $state<string[]>(loadRecents())

  function loadRecents(): string[] {
    try {
      const raw = localStorage.getItem(RECENTS_KEY)
      if (!raw) return []
      const parsed = JSON.parse(raw)
      if (!Array.isArray(parsed)) return []
      return parsed.filter((v): v is string => typeof v === 'string' && normalizeHex(v) !== null).slice(0, MAX_RECENT)
    } catch {
      return []
    }
  }

  function pushRecent(color: string) {
    const norm = normalizeHex(color)
    if (!norm) return
    // De-dup, then unshift to front.
    const next = [norm, ...recents.filter(c => c !== norm)].slice(0, MAX_RECENT)
    recents = next
    try { localStorage.setItem(RECENTS_KEY, JSON.stringify(next)) } catch { /* quota — fine to drop */ }
  }

  function normalizeHex(v: string): string | null {
    const m = /^#?([0-9a-f]{6})$/i.exec(v.trim())
    return m ? `#${m[1].toLowerCase()}` : null
  }

  function pickSwatch(color: string) {
    current = color
    hexInput = color
  }

  function applyAndClose(color: string) {
    pushRecent(color)
    onApply(color)
    onClose()
  }

  function handleHexInput(e: Event) {
    const v = (e.target as HTMLInputElement).value
    hexInput = v
    const norm = normalizeHex(v)
    if (norm) current = norm
  }

  function handleApply() {
    applyAndClose(current)
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('color-picker-backdrop')) {
      onClose()
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      handleApply()
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="color-picker-backdrop" role="presentation" onclick={handleBackdropClick} out:fade={{ duration: 150 }}>
  <div class="color-picker" role="dialog" aria-modal="true" aria-label={t('color.title')}>
    <div class="cp-header">
      <span class="cp-title">{t('color.title')}</span>
      <button class="cp-close" onclick={onClose} title={t('common.close')}><X size={16} /></button>
    </div>

    <div class="cp-body">
      <div class="cp-preview-row">
        <div class="cp-preview" style="background:{current}" aria-label={t('color.current')}></div>
        <input
          type="text"
          class="cp-hex"
          value={hexInput}
          oninput={handleHexInput}
          spellcheck="false"
          maxlength="7"
          use:focusOnMount
        />
      </div>

      {#if recents.length > 0}
        <div class="cp-recents" role="group" aria-label={t('color.recents')}>
          <div class="cp-section-label">{t('color.recents')}</div>
          <div class="cp-recents-row">
            {#each recents as color}
              <button
                class="cp-swatch cp-recent-swatch"
                class:selected={color === current}
                style="background:{color}"
                title={color}
                onclick={() => pickSwatch(color)}
                ondblclick={() => applyAndClose(color)}
              ></button>
            {/each}
          </div>
        </div>
      {/if}

      <div class="cp-palette" role="grid" aria-label={t('color.palette')}>
        {#each PALETTE as row}
          <div class="cp-row" role="row">
            {#each row as color}
              <button
                class="cp-swatch"
                class:selected={color === current}
                style="background:{color}"
                title={color}
                role="gridcell"
                onclick={() => pickSwatch(color)}
                ondblclick={() => applyAndClose(color)}
              ></button>
            {/each}
          </div>
        {/each}
      </div>
    </div>

    <div class="cp-footer">
      <button class="btn-secondary" onclick={onClose}>{t('common.close')}</button>
      <button class="btn-primary" onclick={handleApply}>{t('common.apply')}</button>
    </div>
  </div>
</div>

<style>
  .color-picker-backdrop {
    position: fixed;
    inset: 0;
    z-index: 1100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.4);
    animation: fade-in var(--duration-normal) var(--ease-out);
  }
  .color-picker {
    width: 340px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    display: flex;
    flex-direction: column;
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }
  .cp-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
  }
  .cp-title {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-primary);
  }
  .cp-close {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    display: inline-flex;
  }
  .cp-close:hover {
    color: var(--text-primary);
  }
  .cp-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .cp-preview-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .cp-preview {
    width: 44px;
    height: 44px;
    border-radius: 6px;
    border: 1px solid var(--border);
    flex-shrink: 0;
  }
  .cp-hex {
    flex: 1;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-main);
    color: var(--text-primary);
    font-family: var(--font-mono, ui-monospace, SFMono-Regular, monospace);
    font-size: 0.85rem;
    text-transform: lowercase;
    outline: none;
  }
  .cp-hex:focus {
    border-color: var(--accent);
  }
  .cp-recents {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .cp-section-label {
    font-size: 0.65rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .cp-recents-row {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .cp-recent-swatch {
    flex: 0 0 auto;
    width: 22px;
    height: 22px;
    aspect-ratio: auto;
  }
  .cp-palette {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .cp-row {
    display: flex;
    gap: 4px;
  }
  .cp-swatch {
    flex: 1;
    aspect-ratio: 1;
    min-width: 0;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 4px;
    cursor: pointer;
    transition: transform 0.08s, box-shadow 0.08s;
  }
  .cp-swatch:hover {
    transform: scale(1.08);
    z-index: 1;
  }
  .cp-swatch.selected {
    box-shadow: 0 0 0 2px var(--bg-surface), 0 0 0 4px var(--accent);
    z-index: 2;
  }
  .cp-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 10px 12px;
    border-top: 1px solid var(--border);
  }
  .btn-secondary,
  .btn-primary {
    padding: 6px 14px;
    border-radius: 6px;
    font-size: 0.82rem;
    cursor: pointer;
    border: 1px solid var(--border);
  }
  .btn-secondary {
    background: var(--bg-main);
    color: var(--text-primary);
  }
  .btn-secondary:hover {
    background: var(--bg-hover);
  }
  .btn-primary {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .btn-primary:hover {
    filter: brightness(1.08);
  }
</style>
