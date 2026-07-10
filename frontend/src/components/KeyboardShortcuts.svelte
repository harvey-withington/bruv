<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { X, Search } from 'lucide-svelte'

  let { onClose }: { onClose: () => void } = $props()

  let filter = $state('')
  let filterInput: HTMLInputElement | undefined = $state()

  type ShortcutGroup = {
    titleKey: string
    shortcuts: { keys: string[]; descKey: string }[]
  }

  const groups: ShortcutGroup[] = [
    {
      titleKey: 'shortcuts.group_global',
      shortcuts: [
        { keys: ['?'], descKey: 'shortcuts.show_shortcuts' },
        { keys: ['/'], descKey: 'shortcuts.focus_search' },
        { keys: ['Esc'], descKey: 'shortcuts.close_dialog' },
      ],
    },
    {
      titleKey: 'shortcuts.group_board',
      shortcuts: [
        { keys: ['N'], descKey: 'shortcuts.new_card' },
        { keys: ['P'], descKey: 'shortcuts.toggle_project_chat' },
        { keys: ['Ctrl', 'Drag'], descKey: 'shortcuts.duplicate_card' },
      ],
    },
    {
      titleKey: 'shortcuts.group_card',
      shortcuts: [
        { keys: ['Esc'], descKey: 'shortcuts.close_card' },
        { keys: ['Enter'], descKey: 'shortcuts.save_field' },
      ],
    },
    {
      titleKey: 'shortcuts.group_chat',
      shortcuts: [
        { keys: ['Enter'], descKey: 'shortcuts.send_message' },
        { keys: ['Shift', 'Enter'], descKey: 'shortcuts.newline' },
      ],
    },
  ]

  let filteredGroups = $derived(
    filter.trim() === ''
      ? groups
      : groups
          .map(g => ({
            ...g,
            shortcuts: g.shortcuts.filter(s =>
              t(s.descKey).toLowerCase().includes(filter.toLowerCase()) ||
              s.keys.join('+').toLowerCase().includes(filter.toLowerCase())
            ),
          }))
          .filter(g => g.shortcuts.length > 0)
  )

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
    }
  }

  $effect(() => {
    filterInput?.focus()
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="shortcuts-overlay" onclick={onClose} onkeydown={handleKeydown} out:fade={{ duration: 150 }}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="shortcuts-panel" onclick={(e) => e.stopPropagation()}>
    <div class="shortcuts-header">
      <h2>{t('shortcuts.title')}</h2>
      <div class="search-box">
        <Search size={14} />
        <input
          bind:this={filterInput}
          type="text"
          placeholder={t('shortcuts.search_placeholder')}
          bind:value={filter}
        />
      </div>
      <button class="close-btn" onclick={onClose}><X size={16} /></button>
    </div>

    <div class="shortcuts-body">
      {#each filteredGroups as group}
        <div class="shortcut-group">
          <h3>{t(group.titleKey)}</h3>
          {#each group.shortcuts as shortcut}
            <div class="shortcut-row">
              <span class="shortcut-desc">{t(shortcut.descKey)}</span>
              <span class="shortcut-keys">
                {#each shortcut.keys as key, i}
                  {#if i > 0}<span class="key-sep">+</span>{/if}
                  <kbd>{key === 'Drag' ? t('shortcuts.key_drag') : key}</kbd>
                {/each}
              </span>
            </div>
          {/each}
        </div>
      {/each}
      {#if filteredGroups.length === 0}
        <p class="no-results">{t('shortcuts.no_results')}</p>
      {/if}
    </div>
  </div>
</div>

<style>
  .shortcuts-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    z-index: 950;
    display: flex;
    align-items: center;
    justify-content: center;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .shortcuts-panel {
    background: var(--bg-base);
    border: 1px solid var(--border-muted);
    border-radius: 10px;
    width: min(520px, 90vw);
    max-height: 70vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .shortcuts-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .shortcuts-header h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--text-strong);
    white-space: nowrap;
  }

  .search-box {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1;
    padding: 0.3rem 0.5rem;
    border: 1px solid var(--border-muted);
    border-radius: 5px;
    background: var(--bg-surface);
    color: var(--text-muted);
    transition: border-color var(--duration-normal);
  }
  .search-box:focus-within {
    border-color: var(--accent);
  }
  .search-box input {
    border: none;
    background: none;
    outline: none;
    color: var(--text-body);
    font-size: 0.8rem;
    width: 100%;
  }

  .close-btn {
    display: flex;
    align-items: center;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    transition: color var(--duration-normal), background var(--duration-normal);
  }
  .close-btn:hover {
    color: var(--text-strong);
    background: var(--bg-subtle-hover);
  }

  .shortcuts-body {
    overflow-y: auto;
    padding: 0.75rem 1rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .shortcut-group h3 {
    margin: 0 0 0.4rem 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
  }

  .shortcut-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.3rem 0;
  }

  .shortcut-desc {
    font-size: 0.8rem;
    color: var(--text-body);
  }

  .shortcut-keys {
    display: flex;
    align-items: center;
    gap: 0.2rem;
  }

  kbd {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 24px;
    height: 22px;
    padding: 0 0.4rem;
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    background: var(--bg-elevated);
    color: var(--text-muted);
    font-size: 0.68rem;
    font-family: inherit;
    font-weight: 500;
    box-shadow: 0 1px 0 var(--border-muted);
  }

  .key-sep {
    font-size: 0.65rem;
    color: var(--text-faint);
  }

  .no-results {
    text-align: center;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 1rem;
  }
</style>
