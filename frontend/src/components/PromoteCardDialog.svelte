<script lang="ts">
  // Promote a card into its own project: pick a Stream, name the project
  // (prefilled from the card's title), and the backend creates the project
  // with its default category and pins the card there. The card is
  // referenced, not moved — existing pins stay.
  import { onMount } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { fade } from 'svelte/transition'
  import { showToast } from '../lib/toast.svelte'
  import { focusOnMount, focusTrap, portal } from '../lib/actions'
  import { draggable } from '../lib/draggable'
  import { X } from 'lucide-svelte'
  import { ListBrands, ListStreams, PromoteCardToProject } from '@shared/api'
  import { nav } from '../lib/store.svelte'
  import type { Brand, Stream } from '@shared/types'

  let { cardId, cardTitle, hasDescription, onClose, onPromoted }: {
    cardId: string
    cardTitle: string
    /** Whether the card has a description worth offering to copy. */
    hasDescription: boolean
    onClose: () => void
    /** Called after a successful promotion, right before navigating to the
     *  new project — the parent should close the card modal here. */
    onPromoted: () => void
  } = $props()

  // Captured once on open — the dialog is remounted per-open.
  /* svelte-ignore state_referenced_locally */
  let name = $state(cardTitle)
  let brands = $state<Brand[]>([])
  let streams = $state<Stream[]>([])
  let brandSlug = $state('')
  let streamSlug = $state('')
  let copyDescription = $state(false)
  let loading = $state(true)
  let saving = $state(false)

  onMount(async () => {
    try {
      brands = (await ListBrands()) || []
      brandSlug = brands.some(b => b.slug === nav.brandSlug) ? nav.brandSlug! : (brands[0]?.slug ?? '')
      if (brandSlug) await loadStreams(brandSlug, nav.streamSlug ?? undefined)
    } catch {
      showToast(t('promote.err_load'), 'error')
    } finally {
      loading = false
    }
  })

  async function loadStreams(forBrand: string, preferred?: string) {
    streams = (await ListStreams(forBrand)) || []
    streamSlug = streams.some(s => s.slug === preferred) ? preferred! : (streams[0]?.slug ?? '')
  }

  async function onBrandChange() {
    try {
      await loadStreams(brandSlug)
    } catch {
      streams = []
      streamSlug = ''
      showToast(t('promote.err_load'), 'error')
    }
  }

  const canPromote = $derived(!loading && !saving && !!name.trim() && !!brandSlug && !!streamSlug)

  async function promote() {
    if (!canPromote) return
    saving = true
    try {
      const res = await PromoteCardToProject(cardId, brandSlug, streamSlug, name.trim(), copyDescription)
      showToast(t('promote.success', { name: res.project.name }), 'success')
      onPromoted()
      document.dispatchEvent(new CustomEvent('bruv:select-project', {
        detail: { brandSlug, streamSlug, projectSlug: res.project.slug },
      }))
    } catch (e) {
      const detail = e instanceof Error && e.message ? `: ${e.message}` : ''
      showToast(t('promote.err') + detail, 'error')
    } finally {
      saving = false
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
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
    aria-label={t('promote.title')}
    use:draggable={{ handle: '.dialog-header' }}
    use:focusTrap
  >
    <div class="dialog-header">
      <h2>{t('promote.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <p class="intro">{t('promote.intro')}</p>

      <div class="field-row">
        <span class="field-label">{t('promote.name')}</span>
        <input
          class="field-input"
          bind:value={name}
          use:focusOnMount={true}
          placeholder={t('promote.name_placeholder')}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); promote() } }}
        />
      </div>

      <div class="field-row">
        <span class="field-label">{t('promote.stream')}</span>
        {#if loading}
          <span class="hint">{t('common.loading')}</span>
        {:else if brands.length === 0}
          <span class="hint">{t('promote.no_streams')}</span>
        {:else}
          <div class="picker-row">
            <select class="field-select" bind:value={brandSlug} onchange={onBrandChange} aria-label={t('promote.brand')}>
              {#each brands as b (b.id)}
                <option value={b.slug}>{b.name}</option>
              {/each}
            </select>
            <select class="field-select" bind:value={streamSlug} disabled={streams.length === 0} aria-label={t('promote.stream')}>
              {#each streams as s (s.id)}
                <option value={s.slug}>{s.name}</option>
              {/each}
            </select>
          </div>
          {#if streams.length === 0}
            <span class="hint">{t('promote.no_streams')}</span>
          {/if}
        {/if}
      </div>

      {#if hasDescription}
        <label class="check-row">
          <input type="checkbox" bind:checked={copyDescription} />
          <span>{t('promote.copy_description')}</span>
        </label>
      {/if}
    </div>

    <div class="dialog-footer">
      <button class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
      <button class="btn-primary" onclick={promote} disabled={!canPromote}>
        {saving ? t('common.working') : t('promote.action')}
      </button>
    </div>
  </div>
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
    width: 400px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
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
  .close-btn:hover, .close-btn:focus-visible { color: var(--text-primary); }
  .dialog-body { padding: 1.25rem; overflow-y: auto; flex: 1; display: flex; flex-direction: column; gap: 0.85rem; min-height: 0; }
  .dialog-footer { padding: 0.75rem 1.25rem; border-top: 1px solid var(--border-muted); display: flex; justify-content: flex-end; gap: 0.5rem; }

  .intro { margin: 0; font-size: 0.8rem; color: var(--text-muted); }

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

  .picker-row { display: flex; gap: 0.5rem; }
  .field-select {
    flex: 1;
    min-width: 0;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.45rem 0.4rem;
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
  }
  .field-select:focus { border-color: var(--accent); }
  .field-select:disabled { opacity: 0.5; }

  .check-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    cursor: pointer;
  }
  .check-row input { cursor: pointer; }

  .hint { font-size: 0.75rem; color: var(--text-muted); }

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
  .btn-primary:hover, .btn-primary:focus-visible { opacity: 0.9; }
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
  .btn-secondary:hover, .btn-secondary:focus-visible { background: var(--bg-hover); }
</style>
