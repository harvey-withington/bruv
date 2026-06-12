<script lang="ts">
  import { ListCardComments } from '@shared/api'
  import { Share2, Copy, Download, FileJson } from 'lucide-svelte'
  import { cardToMarkdown } from '@shared/cardMarkdown'
  import { downloadBlob, sanitizeFilenameStem } from '@shared/download'
  import { buildCardExportPayload, cardMarkdownLabels } from '../lib/cardExport'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { floatingDropdown } from '../lib/actions'
  import type { Card } from '@shared/types'

  // Share/export menu for the card dialog footer: copy as Markdown,
  // save as Markdown file, save as BRUV JSON. Self-contained — owns
  // its open state and click-outside handling.

  let { card }: { card: Card } = $props()

  let open = $state(false)
  let btnEl = $state<HTMLButtonElement | null>(null)

  function handleWindowClick(e: MouseEvent) {
    if (!open) return
    const target = e.target as Node
    if (!btnEl?.contains(target) && !(target as HTMLElement).closest?.('.share-menu')) open = false
  }

  // Build the markdown snapshot, loading comments lazily — comments
  // aren't kept in `card`, so we fetch on demand rather than eagerly.
  async function buildCardMarkdown(): Promise<string> {
    let comments: Awaited<ReturnType<typeof ListCardComments>> = []
    try { comments = await ListCardComments(card.id) }
    catch { /* comments are optional in the export — fall through with empty */ }
    return cardToMarkdown(card, { comments, untitledLabel: t('card.untitled'), labels: cardMarkdownLabels() })
  }

  async function copyCardAsMarkdown() {
    open = false
    const md = await buildCardMarkdown()
    try {
      await navigator.clipboard.writeText(md)
      showToast(t('card.copy_markdown_done'), 'success')
    } catch {
      showToast(t('card.copy_markdown_error'), 'error')
    }
  }

  async function exportCardAsMarkdown() {
    open = false
    const md = await buildCardMarkdown()
    downloadBlob(md, `${sanitizeFilenameStem(card.title)}.md`, 'text/markdown;charset=utf-8')
    showToast(t('card.export_markdown_done'), 'success')
  }

  async function exportCardAsJson() {
    open = false
    try {
      const payload = await buildCardExportPayload(card)
      const json = JSON.stringify(payload, null, 2)
      downloadBlob(json, `${sanitizeFilenameStem(card.title)}.bruv-card.json`, 'application/json;charset=utf-8')
      showToast(t('card.export_json_done'), 'success')
    } catch {
      showToast(t('card.export_json_error'), 'error')
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<button
  class="btn-share"
  bind:this={btnEl}
  onclick={() => open = !open}
  title={t('tooltip.share_card')}
>
  <Share2 size={14} /> {t('card.share')}
</button>
{#if open && btnEl}
  <div class="share-menu" use:floatingDropdown={{ trigger: btnEl }}>
    <button class="share-menu-item" onclick={copyCardAsMarkdown}>
      <Copy size={14} />
      <span>{t('card.copy_markdown')}</span>
    </button>
    <button class="share-menu-item" onclick={exportCardAsMarkdown}>
      <Download size={14} />
      <span>{t('card.export_markdown')}</span>
    </button>
    <button class="share-menu-item" onclick={exportCardAsJson}>
      <FileJson size={14} />
      <span>{t('card.export_json')}</span>
    </button>
  </div>
{/if}

<style>
  .btn-share {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.3rem 0.7rem;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted);
    font-size: 0.8rem;
    cursor: pointer;
  }
  .btn-share:hover { color: var(--text-primary); background: var(--bg-elevated); }

  /* :global because floatingDropdown re-parents the menu out of this
     component's scope boundary. */
  :global(.share-menu) {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 16px var(--shadow-lg);
    padding: 0.35rem;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 220px;
  }
  :global(.share-menu .share-menu-item) {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-body);
    font-size: 0.8rem;
    cursor: pointer;
    text-align: left;
    white-space: nowrap;
  }
  :global(.share-menu .share-menu-item:hover) { background: var(--bg-elevated); color: var(--text-primary); }
</style>
