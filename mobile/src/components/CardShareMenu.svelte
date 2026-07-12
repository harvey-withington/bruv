<script lang="ts">
  import { Share2, Copy, Download, FileJson, Loader2 } from 'lucide-svelte'
  import { repoRPC } from '../lib/auth'
  import { cardToMarkdown } from '@shared/cardMarkdown'
  import { downloadBlob, sanitizeFilenameStem } from '@shared/download'
  import { buildCardExportPayload, cardMarkdownLabels } from '../lib/cardExport'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import type { Card, CardComment } from '@shared/types'

  // Share/export dropdown for the card footer — mobile twin of the
  // desktop CardShareMenu: one Share trigger opening a compact menu with
  // copy-as-Markdown / save-as-Markdown / save-as-JSON. Open state is
  // bindable so CardPage can guard its own Back/Escape layers while the
  // menu is up. While open, the menu owns one history entry (same
  // push/pop pattern as the pickers and sheets) so hardware/gesture Back
  // closes the menu and leaves the card page put; backdrop tap and
  // Escape close it too, popping that entry back off.

  let { card, open = $bindable(false) }: { card: Card; open?: boolean } = $props()

  // JSON export fetches + base64-encodes every attachment — seconds on
  // big cards. The trigger doubles as the busy indicator meanwhile.
  let exporting = $state(false)

  $effect(() => {
    if (!open) return
    history.pushState({ cardShareMenu: true }, '')
    const onPop = () => { open = false }
    window.addEventListener('popstate', onPop)
    return () => {
      window.removeEventListener('popstate', onPop)
      if (history.state?.cardShareMenu) history.back()
    }
  })

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!open) return
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      open = false
    }
  }

  // Build the markdown snapshot, loading comments lazily — comments
  // aren't kept in `card`, so we fetch on demand rather than eagerly.
  async function buildMarkdown(): Promise<string> {
    let comments: CardComment[] = []
    try { comments = (await repoRPC<CardComment[]>('ListCardComments', [card.id])) ?? [] }
    catch { /* comments are optional in the export — fall through with empty */ }
    return cardToMarkdown(card, { comments, untitledLabel: t('card.untitled'), labels: cardMarkdownLabels() })
  }

  async function copyCardAsMarkdown() {
    open = false
    const md = await buildMarkdown()
    try {
      await navigator.clipboard.writeText(md)
      showToast(t('card.copy_markdown_done'), 'success')
    } catch {
      showToast(t('card.copy_markdown_error'), 'error')
    }
  }

  async function exportCardAsMarkdown() {
    open = false
    const md = await buildMarkdown()
    downloadBlob(md, `${sanitizeFilenameStem(card.title)}.md`, 'text/markdown;charset=utf-8')
    showToast(t('card.export_markdown_done'), 'success')
  }

  async function exportCardAsJson() {
    open = false
    exporting = true
    try {
      const payload = await buildCardExportPayload(card)
      const json = JSON.stringify(payload, null, 2)
      downloadBlob(json, `${sanitizeFilenameStem(card.title)}.bruv-card.json`, 'application/json;charset=utf-8')
      showToast(t('card.export_json_done'), 'success')
    } catch {
      showToast(t('card.export_json_error'), 'error')
    } finally {
      exporting = false
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<div class="share-wrap">
  <button
    type="button"
    class="share-btn"
    onclick={() => (open = !open)}
    disabled={exporting}
    aria-haspopup="menu"
    aria-expanded={open}
  >
    {#if exporting}
      <Loader2 size={14} class="spin" />
      {t('common.working')}
    {:else}
      <Share2 size={14} />
      {t('card.share')}
    {/if}
  </button>

  {#if open}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="backdrop" onclick={() => (open = false)}></div>
    <div class="menu" role="menu" aria-label={t('card.share')}>
      <button type="button" class="menu-item" role="menuitem" onclick={copyCardAsMarkdown}>
        <Copy size={16} />
        <span>{t('card.copy_markdown')}</span>
      </button>
      <button type="button" class="menu-item" role="menuitem" onclick={exportCardAsMarkdown}>
        <Download size={16} />
        <span>{t('card.export_markdown')}</span>
      </button>
      <button type="button" class="menu-item" role="menuitem" onclick={exportCardAsJson}>
        <FileJson size={16} />
        <span>{t('card.export_json')}</span>
      </button>
    </div>
  {/if}
</div>

<style>
  .share-wrap {
    position: relative;
    display: inline-flex;
  }

  .share-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    min-height: 40px;
    touch-action: manipulation;
  }
  .share-btn:hover,
  .share-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
  .share-btn:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .share-btn :global(.spin) {
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* Invisible tap-catcher under the menu — outside-tap closes without
     activating whatever was underneath. */
  .backdrop {
    position: fixed;
    inset: 0;
    background: transparent;
    z-index: 60;
  }

  /* Compact dropdown, opening upward — the trigger lives in the page
     footer, so down would run off-screen. Right-aligned to the trigger. */
  .menu {
    position: absolute;
    bottom: calc(100% + 4px);
    right: 0;
    z-index: 61;
    min-width: 220px;
    background: var(--bg-elev-1, var(--bg));
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    padding: 0.25rem;
    display: flex;
    flex-direction: column;
    animation: menu-in 140ms ease forwards;
  }

  .menu-item {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    background: transparent;
    border: none;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    text-align: left;
    cursor: pointer;
    padding: 0.6rem 0.75rem;
    border-radius: 8px;
    min-height: 44px;
    touch-action: manipulation;
  }
  .menu-item:hover,
  .menu-item:focus-visible {
    background: var(--bg-elev-2, var(--border));
    outline: none;
  }

  @keyframes menu-in {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @media (prefers-reduced-motion: reduce) {
    .menu { animation: none; }
  }
</style>
