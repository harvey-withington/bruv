<script lang="ts">
  // Modal that lists known remote connections + lets the user add or
  // remove them and switch the active one. The implicit "Local" entry
  // is rendered first and never deletable.
  //
  // Switching connections triggers a window reload (handled inside the
  // store helper), so we don't bother re-fetching state in-place.

  import { fade } from 'svelte/transition'
  import { X, Plus, Check, Trash2, Server, Monitor, ArrowRightLeft } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap } from '../lib/actions'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { connections, removeConnection, switchConnection, addConnection } from '../lib/connections.svelte'
  import AddConnectionForm from './AddConnectionForm.svelte'

  let { onClose }: { onClose: () => void } = $props()

  let mode = $state<'list' | 'add'>('list')

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (mode === 'add') {
        mode = 'list'
        return
      }
      onClose()
    }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  async function activate(id: string) {
    if (id === connections.active) {
      // Switching to the already-active entry is a no-op; close instead.
      onClose()
      return
    }
    try {
      await switchConnection(id)
      // Reload pending — UI freezes here until the page swap.
    } catch (err) {
      showToast(t('connection.switch_failed'), 'error')
      console.error('switchConnection failed:', err)
    }
  }

  async function activateLocal() {
    if (!connections.active) {
      onClose()
      return
    }
    try {
      await switchConnection('')
    } catch (err) {
      showToast(t('connection.switch_failed'), 'error')
      console.error('switchConnection(local) failed:', err)
    }
  }

  async function handleRemove(id: string, name: string) {
    const ok = await showConfirm(t('connection.remove_confirm').replace('{name}', name))
    if (!ok) return
    try {
      await removeConnection(id)
      showToast(t('connection.removed'), 'success')
    } catch (err) {
      showToast(t('connection.remove_failed'), 'error')
      console.error('removeConnection failed:', err)
    }
  }

  async function handleEnrolled(args: { name: string; url: string; deviceToken: string }) {
    try {
      await addConnection(args.name, args.url, args.deviceToken, { activate: true })
      // activate: true triggers a reload; nothing more to do.
    } catch (err) {
      showToast((err as Error).message || t('connection.add_failed'), 'error')
      console.error('addConnection failed:', err)
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="overlay" role="presentation" onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" use:focusTrap>
    <div class="dialog-header">
      <h2>{mode === 'add' ? t('connection.add_title') : t('connection.dialog_title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      {#if mode === 'list'}
        <p class="subtitle">{t('connection.dialog_subtitle')}</p>

        <ul class="connection-list">
          <!-- Local is always first, always non-deletable. -->
          <li class="connection-item" class:active={!connections.active}>
            <div class="conn-icon"><Monitor size={16} /></div>
            <div class="conn-info">
              <div class="conn-name">{t('connection.local_label')}</div>
              <div class="conn-url">{t('connection.local_hint')}</div>
            </div>
            {#if !connections.active}
              <span class="active-badge"><Check size={12} /> {t('connection.active')}</span>
            {:else}
              <button class="action-btn" onclick={activateLocal} title={t('connection.switch_to')}>
                <ArrowRightLeft size={14} />
              </button>
            {/if}
          </li>

          {#each connections.connections as c (c.id)}
            <li class="connection-item" class:active={c.id === connections.active}>
              <div class="conn-icon"><Server size={16} /></div>
              <div class="conn-info">
                <div class="conn-name">{c.name}</div>
                <div class="conn-url">{c.url}</div>
              </div>
              {#if c.id === connections.active}
                <span class="active-badge"><Check size={12} /> {t('connection.active')}</span>
              {:else}
                <button class="action-btn" onclick={() => activate(c.id)} title={t('connection.switch_to')}>
                  <ArrowRightLeft size={14} />
                </button>
              {/if}
              <button class="action-btn danger" onclick={() => handleRemove(c.id, c.name)} title={t('connection.remove')}>
                <Trash2 size={14} />
              </button>
            </li>
          {/each}
        </ul>

        <button class="add-btn" onclick={() => { mode = 'add' }}>
          <Plus size={14} />
          {t('connection.add_action')}
        </button>
      {:else}
        <p class="subtitle">{t('connection.add_subtitle')}</p>
        <AddConnectionForm
          onEnrolled={handleEnrolled}
          onCancel={() => { mode = 'list' }}
          submitLabel={t('connection.add_and_switch')}
        />
      {/if}
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    display: grid;
    place-items: center;
    z-index: 1000;
    padding: 1.5rem;
  }

  .dialog {
    width: 100%;
    max-width: 540px;
    background: var(--bg-surface, var(--bg-base));
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.4);
    display: flex;
    flex-direction: column;
    max-height: 85vh;
    overflow: hidden;
  }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.85rem 1.1rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1rem;
    color: var(--text-strong);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
  }
  .close-btn:hover { color: var(--text-strong); background: var(--bg-elevated); }

  .dialog-body {
    padding: 1rem 1.1rem 1.1rem;
    overflow-y: auto;
  }

  .subtitle {
    margin: 0 0 0.85rem;
    color: var(--text-secondary);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .connection-list {
    list-style: none;
    margin: 0 0 1rem;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .connection-item {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    padding: 0.65rem 0.75rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
  }
  .connection-item.active {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 8%, var(--bg-elevated));
  }

  .conn-icon {
    color: var(--text-muted);
    display: flex;
    align-items: center;
  }
  .connection-item.active .conn-icon {
    color: var(--accent);
  }

  .conn-info {
    flex: 1;
    min-width: 0;
  }

  .conn-name {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-strong);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conn-url {
    font-size: 0.7rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .active-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--accent);
    padding: 0.15rem 0.45rem;
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    border-radius: 999px;
  }

  .action-btn {
    background: none;
    border: 1px solid transparent;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.35rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .action-btn:hover {
    color: var(--text-strong);
    background: var(--bg-base);
    border-color: var(--border);
  }
  .action-btn.danger:hover {
    color: var(--danger-light, var(--danger));
    border-color: var(--danger);
  }

  .add-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.45rem 0.85rem;
    background: transparent;
    color: var(--accent);
    border: 1px dashed var(--accent);
    border-radius: 6px;
    cursor: pointer;
    font: inherit;
    font-weight: 500;
  }
  .add-btn:hover {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
  }
</style>
