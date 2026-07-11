<script lang="ts">
  import { ListCardComments } from '@shared/api'
  import { Share2, Copy, Download, FileJson, Loader2 } from 'lucide-svelte'
  import { cardToMarkdown } from '@shared/cardMarkdown'
  import { downloadBlob, sanitizeFilenameStem } from '@shared/download'
  import { buildCardExportPayload, cardMarkdownLabels } from '../lib/cardExport'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { floatingDropdown, clickOutside } from '../lib/actions'
  import type { Card } from '@shared/types'

  // Share/export menu for the card dialog footer: copy as Markdown,
  // save as Markdown file, save as BRUV JSON. Self-contained — owns
  // its open state and click-outside/Escape handling.

  let { card }: { card: Card } = $props()

  let open = $state(false)
  let btnEl = $state<HTMLButtonElement | null>(null)
  // JSON export fetches + base64-encodes every attachment — seconds on
  // big cards. The trigger doubles as the busy indicator meanwhile.
  let exporting = $state(false)

  function handleKeydown(e: KeyboardEvent) {
    if (!open) return
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      open = false
    }
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

<svelte:window onkeydown={handleKeydown} />

<button
  class="btn-share"
  bind:this={btnEl}
  onclick={() => open = !open}
  disabled={exporting}
  title={exporting ? t('card.exporting') : t('tooltip.share_card')}
>
  {#if exporting}
    <Loader2 size={14} class="spin" />
  {:else}
    <Share2 size={14} />
  {/if}
  {t('card.share')}
</button>
{#if open && btnEl}
  <div
    class="dropdown-menu"
    use:floatingDropdown={{ trigger: btnEl }}
    use:clickOutside={{ onOutsideClick: () => open = false, exclude: [btnEl] }}
  >
    <button class="dropdown-menu-item" onclick={copyCardAsMarkdown}>
      <Copy size={14} />
      <span>{t('card.copy_markdown')}</span>
    </button>
    <button class="dropdown-menu-item" onclick={exportCardAsMarkdown}>
      <Download size={14} />
      <span>{t('card.export_markdown')}</span>
    </button>
    <button class="dropdown-menu-item" onclick={exportCardAsJson}>
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
  .btn-share:hover,
  .btn-share:focus-visible {
    color: var(--text-primary);
    background: var(--bg-elevated);
  }
  .btn-share:disabled {
    cursor: default;
    background: transparent;
  }
  .btn-share :global(.spin) {
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* Menu chrome lives in the shared .dropdown-menu / .dropdown-menu-item
     classes (style.css) — :global there too, since floatingDropdown
     re-parents the menu out of this component's scope boundary. */
</style>
