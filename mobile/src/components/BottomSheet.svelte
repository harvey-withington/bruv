<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import { fly, fade } from 'svelte/transition'
  import { X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  // Slide-up sheet chrome: backdrop, panel, titled header with a close
  // button — plus the three ways a mobile sheet gets dismissed (tap the
  // backdrop, Escape, Back gesture). Content goes in as a snippet.
  //
  // `historyKey` names the history entry the sheet owns so a Back
  // gesture closes it instead of leaving the page underneath.

  let {
    title,
    subtitle = '',
    historyKey,
    onClose,
    children,
  }: {
    title: string
    subtitle?: string
    historyKey: string
    onClose: () => void
    children: Snippet
  } = $props()

  const titleID = $derived(`sheet-title-${historyKey}`)

  onMount(() => {
    history.pushState({ [historyKey]: true }, '')
    // Close only when OUR entry was the one popped. A DESCENDANT
    // sheet's cleanup also fires popstate (its history.back() lands
    // back on our entry, so our key is current again) — reacting to
    // that cascaded a nested sheet's close into ours, eating two
    // entries per tap and tearing down a dialog the user never
    // dismissed.
    const onPop = () => {
      if (!history.state?.[historyKey]) onClose()
    }
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.[historyKey]) history.back()
    }
  })

  function onBackdrop(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function onKey(e: KeyboardEvent) {
    if (e.key !== 'Escape') return
    // Only the TOPMOST sheet may react — with nested sheets every
    // instance hears the same window keydown, and one Escape used to
    // close them all. The topmost sheet is the one whose synthetic
    // entry is current. Deliberately bubble-phase: ConfirmDialog's
    // capture-phase shield keeps owning Escape when layered above us.
    if (!history.state?.[historyKey]) return
    e.preventDefault()
    onClose()
  }
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="backdrop" role="presentation" onclick={onBackdrop} transition:fade={{ duration: 120 }}>
  <div
    class="sheet"
    transition:fly={{ y: 400, duration: 220 }}
    role="dialog"
    aria-modal="true"
    aria-labelledby={titleID}
  >
    <header>
      <h2 id={titleID}>{title}</h2>
      <button type="button" class="close" onclick={onClose} aria-label={t('common.cancel')}>
        <X size={18} />
      </button>
    </header>
    {#if subtitle}
      <p class="subtitle">{subtitle}</p>
    {/if}
    {@render children()}
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    z-index: 100;
    display: flex;
    align-items: flex-end;
    justify-content: center;
  }

  .sheet {
    width: 100%;
    max-width: 600px;
    max-height: 85vh;
    background: var(--bg-elev-1);
    border-top-left-radius: 16px;
    border-top-right-radius: 16px;
    border-top: 1px solid var(--border);
    border-left: 1px solid var(--border);
    border-right: 1px solid var(--border);
    padding: 1rem 0.85rem 1.25rem;
    padding-bottom: calc(1.25rem + env(safe-area-inset-bottom));
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  header h2 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    color: var(--text);
  }

  .close {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: 6px;
    display: inline-flex;
  }
  .close:hover,
  .close:focus-visible {
    color: var(--text);
    background: var(--bg);
    outline: none;
  }

  .subtitle {
    margin: -0.2rem 0 0.15rem;
    color: var(--text-muted);
    font-size: 0.85rem;
    line-height: 1.4;
  }
</style>
