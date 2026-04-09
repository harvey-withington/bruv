<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { ICON_CATEGORIES } from '../lib/icons'
  import DynamicIcon from './DynamicIcon.svelte'
  import { X } from 'lucide-svelte'
  import { focusOnMount } from '../lib/actions'

  let {
    value = '',
    onSelect,
    onClose
  }: {
    value?: string
    onSelect: (icon: string) => void
    onClose: () => void
  } = $props()

  let search = $state('')

  const filteredCategories = $derived.by(() => {
    if (!search) return ICON_CATEGORIES
    const s = search.toLowerCase()
    const result: Record<string, string[]> = {}
    for (const [cat, icons] of Object.entries(ICON_CATEGORIES)) {
      const filtered = icons.filter(icon => icon.includes(s) || cat.toLowerCase().includes(s))
      if (filtered.length > 0) result[cat] = filtered
    }
    return result
  })

  function handleSelect(icon: string) {
    onSelect(icon)
    onClose()
  }

  function clearIcon() {
    onSelect('')
    onClose()
  }

  function handleBackdropClick(e: MouseEvent) {
    if ((e.target as HTMLElement).classList.contains('icon-picker-backdrop')) {
      onClose()
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="icon-picker-backdrop" role="presentation" onclick={handleBackdropClick}>
  <div class="icon-picker">
    <div class="picker-header">
      <input
        type="text"
        class="picker-search"
        placeholder={t('icon.search')}
        bind:value={search}
        use:focusOnMount
      />
      <button class="picker-close" onclick={onClose} title={t('common.close')}><X size={16} /></button>
    </div>

    {#if value}
      <button class="clear-btn" onclick={clearIcon}>{t('icon.clear')}</button>
    {/if}

    <div class="picker-body">
      {#each Object.entries(filteredCategories) as [category, icons]}
        <div class="category">
          <div class="category-label">{category}</div>
          <div class="icon-grid">
            {#each icons as icon}
              <button
                class="icon-cell"
                class:selected={icon === value}
                onclick={() => handleSelect(icon)}
                title={icon}
              >
                <DynamicIcon name={icon} size={18} />
              </button>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .icon-picker-backdrop {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.3);
  }
  .icon-picker {
    width: 400px;
    max-height: 500px;
    display: flex;
    flex-direction: column;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
  }
  .picker-header {
    display: flex;
    gap: 8px;
    padding: 12px;
    border-bottom: 1px solid var(--border);
  }
  .picker-search {
    flex: 1;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-main);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
  }
  .picker-search:focus {
    border-color: var(--accent);
  }
  .picker-close {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
  }
  .picker-close:hover {
    color: var(--text-primary);
  }
  .clear-btn {
    margin: 8px 12px 0;
    padding: 4px 8px;
    font-size: 0.8em;
    background: none;
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    cursor: pointer;
  }
  .clear-btn:hover {
    color: var(--danger);
    border-color: var(--danger);
  }
  .picker-body {
    overflow-y: auto;
    padding: 8px 12px;
  }
  .category {
    margin-bottom: 12px;
  }
  .category-label {
    font-size: 0.75em;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 6px;
  }
  .icon-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(36px, 1fr));
    gap: 4px;
  }
  .icon-cell {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 6px;
    border: none;
    background: none;
    color: var(--text-primary);
    cursor: pointer;
  }
  .icon-cell:hover {
    background: var(--bg-hover);
  }
  .icon-cell.selected {
    background: var(--accent-bg);
    color: var(--accent);
  }
</style>
