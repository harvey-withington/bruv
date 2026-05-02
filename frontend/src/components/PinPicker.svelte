<script lang="ts">
  import { ListAllCategories } from '@shared/api'
  import { fade } from 'svelte/transition'
  import { X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap } from '../lib/actions'

  type CategoryPath = {
    brandSlug: string; streamSlug: string; projectSlug: string; categorySlug: string
    brandName: string; streamName: string; projectName: string; categoryName: string
    projectId: string; categoryId: string
    breadcrumb: string
    pinnedProjectId?: string
  }

  let { visible, onSelect, onClose }: {
    visible: boolean
    onSelect: (category: CategoryPath) => void
    onClose: () => void
  } = $props()

  let allCategories = $state<CategoryPath[]>([])
  let loading = $state(false)
  let query = $state('')
  let items = $state<CategoryPath[]>([])
  let selectedIndex = $state(0)
  let inputEl = $state<HTMLInputElement | null>(null)
  let debounceTimer: ReturnType<typeof setTimeout> | undefined

  $effect(() => {
    if (visible) {
      query = ''
      items = []
      selectedIndex = 0
      loadCategories()
      setTimeout(() => inputEl?.focus(), 0)
    }
  })

  async function loadCategories() {
    loading = true
    try {
      allCategories = await ListAllCategories() || []
    } catch (e) {
      console.error('PinPicker: failed to load categories', e)
    }
    loading = false
  }

  function handleInput() {
    clearTimeout(debounceTimer)
    const q = query.trim().toLowerCase()
    if (!q) { items = []; selectedIndex = 0; return }
    debounceTimer = setTimeout(() => {
      items = allCategories.filter(c => c.breadcrumb.toLowerCase().includes(q)).slice(0, 12)
      selectedIndex = 0
    }, 100)
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selectedIndex = Math.min(selectedIndex + 1, items.length - 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      selectedIndex = Math.max(selectedIndex - 1, 0)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (items.length > 0) selectItem(items[selectedIndex])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  function selectItem(item: CategoryPath) {
    onSelect(item)
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }
</script>

{#if visible}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="pin-backdrop" onmousedown={handleBackdropClick} out:fade={{ duration: 150 }}>
    <div class="pin-modal" use:focusTrap>
      <div class="pin-header">
        <span class="pin-title">{t('pin.title')}</span>
        <button class="pin-close" onclick={onClose}><X size={16} /></button>
      </div>
      <div class="pin-search">
        <input
          bind:this={inputEl}
          type="text"
          bind:value={query}
          oninput={handleInput}
          onkeydown={handleKeydown}
          placeholder={t('pin.search_placeholder')}
          class="pin-input"
        />
      </div>
      {#if items.length > 0}
        <div class="pin-results" role="listbox">
          {#each items as item, i}
            <button
              class="pin-result"
              class:active={i === selectedIndex}
              role="option"
              aria-selected={i === selectedIndex}
              onmouseenter={() => selectedIndex = i}
              onclick={() => selectItem(item)}
            >
              <span class="pin-breadcrumb">{item.breadcrumb}</span>
            </button>
          {/each}
        </div>
      {:else if loading}
        <div class="pin-empty">{t('pin.loading')}</div>
      {:else if query.trim()}
        <div class="pin-empty">{t('pin.no_results')}</div>
      {:else}
        <div class="pin-empty">{t('pin.start_typing')}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .pin-backdrop {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 6rem;
    z-index: 200;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .pin-modal {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 480px;
    max-width: 92vw;
    max-height: 60vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .pin-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.6rem 0.75rem 0.6rem 1rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .pin-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .pin-close {
    background: none;
    border: none;
    padding: 0.2rem;
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
  }
  .pin-close:hover { color: var(--text-primary); }

  .pin-search {
    padding: 0.5rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .pin-input {
    width: 100%;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    outline: none;
    box-sizing: border-box;
  }
  .pin-input:focus { border-color: var(--accent); }

  .pin-results {
    overflow-y: auto;
    flex: 1;
  }

  .pin-result {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 0.55rem 1rem;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border-muted);
    color: var(--text-body);
    font-size: 0.82rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }
  .pin-result:last-child { border-bottom: none; }
  .pin-result:hover, .pin-result.active { background: var(--bg-elevated); }

  .pin-breadcrumb {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pin-empty {
    padding: 0.75rem 1rem;
    font-size: 0.82rem;
    color: var(--text-muted);
    text-align: center;
  }
</style>
