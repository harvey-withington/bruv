<script lang="ts">
  // Promote a card into its own project: pick a Brand/Stream, name the
  // project (prefilled from the card's title), and the backend creates
  // the project with its default category and pins the card there. The
  // card is referenced, not moved — existing pins stay. Mobile
  // counterpart of desktop's PromoteCardDialog; CardPage has no project
  // context to default from, so brand/stream just default to the first
  // one returned by the server.
  import { onMount } from 'svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { repoRPC } from '../lib/auth'
  import { replace, projectURL } from '../lib/router.svelte'
  import { X } from 'lucide-svelte'
  import type { Brand, Stream, PromotedProject } from '../lib/model'

  let { cardId, cardTitle, hasDescription, onClose }: {
    cardId: string
    cardTitle: string
    /** Whether the card has a description worth offering to copy. */
    hasDescription: boolean
    onClose: () => void
  } = $props()

  // Captured once on open — the sheet is remounted per-open.
  /* svelte-ignore state_referenced_locally */
  let name = $state(cardTitle)
  let brands = $state<Brand[]>([])
  let streams = $state<Stream[]>([])
  let brandSlug = $state('')
  let streamSlug = $state('')
  let copyDescription = $state(false)
  let loading = $state(true)
  let saving = $state(false)

  async function loadStreams(forBrand: string): Promise<void> {
    streams = (await repoRPC<Stream[]>('ListStreams', [forBrand])) ?? []
    streamSlug = streams[0]?.slug ?? ''
  }

  async function load(): Promise<void> {
    loading = true
    try {
      brands = (await repoRPC<Brand[]>('ListBrands', [])) ?? []
      brandSlug = brands[0]?.slug ?? ''
      if (brandSlug) await loadStreams(brandSlug)
    } catch {
      showToast(t('promote.err_load'), 'error')
    } finally {
      loading = false
    }
  }

  async function onBrandChange(): Promise<void> {
    try {
      await loadStreams(brandSlug)
    } catch {
      streams = []
      streamSlug = ''
      showToast(t('promote.err_load'), 'error')
    }
  }

  const canPromote = $derived(!loading && !saving && !!name.trim() && !!brandSlug && !!streamSlug)

  // Set when promotion navigates to the new project: the unmount cleanup
  // must NOT history.back() then — replace() consumes the sheet's synthetic
  // entry instead, so a queued back-traversal can't race the navigation and
  // bounce the user to the card.
  let navigatedAway = false

  async function promote(): Promise<void> {
    if (!canPromote) return
    saving = true
    try {
      const res = await repoRPC<PromotedProject>(
        'PromoteCardToProject',
        [cardId, brandSlug, streamSlug, name.trim(), copyDescription],
      )
      showToast(t('promote.success', { name: res.project.name }), 'success')
      navigatedAway = true
      onClose()
      // Replace the sheet's synthetic history entry with the project URL:
      // Back from the new project returns to the card, and the stack holds
      // no leftover sheet entry.
      replace(projectURL(brandSlug, streamSlug, res.project.slug))
    } catch (e) {
      const detail = e instanceof Error && e.message ? `: ${e.message}` : ''
      showToast(t('promote.err') + detail, 'error')
    } finally {
      saving = false
    }
  }

  function onNameKeydown(e: KeyboardEvent): void {
    // Mobile single-line keyboard contract: Enter commits.
    if (e.key === 'Enter') {
      e.preventDefault()
      void promote()
    }
  }

  function onWindowKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') onClose()
  }

  onMount(() => {
    // Synthetic history entry so hardware/gesture Back closes the sheet
    // instead of the card page underneath it (same pattern as
    // ImportConfirmSheet / CategoryTypesSheet).
    history.pushState({ promote: true }, '')
    const onPop = () => onClose()
    window.addEventListener('popstate', onPop)
    void load()
    return () => {
      window.removeEventListener('popstate', onPop)
      if (!navigatedAway && history.state?.promote) history.back()
    }
  })
</script>

<svelte:window onkeydown={onWindowKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onClose}></div>

<div class="sheet" role="dialog" aria-label={t('promote.title')}>
  <div class="header">
    <span class="grabber" aria-hidden="true"></span>
    <span class="title">{t('promote.title')}</span>
    <button type="button" class="icon-btn" onclick={onClose} aria-label={t('common.cancel')}>
      <X size={18} />
    </button>
  </div>

  <div class="body">
    <p class="intro">{t('promote.intro')}</p>

    <div class="field-row">
      <span class="field-label">{t('promote.name')}</span>
      <input
        class="field-input"
        bind:value={name}
        enterkeyhint="done"
        placeholder={t('promote.name_placeholder')}
        onkeydown={onNameKeydown}
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

  <div class="footer">
    <button type="button" class="btn-secondary" onclick={onClose}>{t('common.cancel')}</button>
    <button type="button" class="btn-primary" disabled={!canPromote} onclick={promote}>
      {saving ? t('common.working') : t('promote.action')}
    </button>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 60;
    animation: fade-in 200ms ease forwards;
  }
  .sheet {
    position: fixed;
    left: 0; right: 0; bottom: 0;
    max-height: 85vh;
    background: var(--bg);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
    z-index: 61;
    display: flex;
    flex-direction: column;
    animation: slide-up 220ms cubic-bezier(0.16, 1, 0.3, 1) forwards;
    padding-bottom: env(safe-area-inset-bottom);
  }
  .header {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 0.85rem 0.6rem;
    border-bottom: 1px solid var(--border);
  }
  .grabber {
    position: absolute;
    top: 6px; left: 50%;
    transform: translateX(-50%);
    width: 36px; height: 4px;
    border-radius: 2px;
    background: var(--text-faint);
    opacity: 0.5;
  }
  .title {
    flex: 1;
    margin-top: 0.4rem;
    font-weight: 600;
    color: var(--text);
    font-size: 0.95rem;
  }
  .icon-btn {
    margin-top: 0.4rem;
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: 6px;
    min-width: 36px;
    min-height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .body {
    padding: 0.75rem 0.85rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .intro {
    margin: 0;
    font-size: 0.82rem;
    color: var(--text-muted);
    line-height: 1.45;
  }

  .field-row { display: flex; flex-direction: column; gap: 0.35rem; }
  .field-label {
    font-size: 0.78rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .field-input {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.6rem 0.75rem;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    min-height: 44px;
    box-sizing: border-box;
  }
  .field-input:focus { outline: none; border-color: var(--accent); }

  .picker-row { display: flex; gap: 0.5rem; }
  .field-select {
    flex: 1;
    min-width: 0;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.55rem 0.5rem;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    min-height: 44px;
  }
  .field-select:focus { outline: none; border-color: var(--accent); }
  .field-select:disabled { opacity: 0.5; }

  .check-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
    color: var(--text);
    min-height: 40px;
    cursor: pointer;
  }
  .check-row input { cursor: pointer; width: 18px; height: 18px; }

  .hint { font-size: 0.78rem; color: var(--text-muted); }

  .footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.6rem 0.85rem;
    border-top: 1px solid var(--border);
  }
  .btn-primary {
    background: var(--accent);
    color: #fff;
    border: none;
    border-radius: 8px;
    padding: 0.55rem 1.1rem;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    min-height: 42px;
  }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-secondary {
    background: none;
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.55rem 1.1rem;
    font-size: 0.9rem;
    color: var(--text);
    cursor: pointer;
    min-height: 42px;
  }

  @keyframes fade-in {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  @keyframes slide-up {
    from { transform: translateY(100%); }
    to { transform: translateY(0); }
  }
  @media (prefers-reduced-motion: reduce) {
    .backdrop, .sheet { animation: fade-in 120ms ease forwards; }
  }
</style>
