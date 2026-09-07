<script lang="ts">
  import { X, ExternalLink, FolderSearch, Pencil, Columns2, Eye, ListTree, Printer } from 'lucide-svelte'
  import type { DocumentLayout } from '@shared/types'
  import { t } from '../../../lib/i18n.svelte'
  import SaveIndicator from '../../SaveIndicator.svelte'

  let { path, layout, onLayout, outline, canOutline, onToggleOutline, saving, onPrint, onOpenExternal, onReveal, onClose }: {
    path: string
    layout: DocumentLayout
    onLayout: (layout: DocumentLayout) => void
    outline: boolean
    /** False when the format has no structure to show — the toggle hides. */
    canOutline: boolean
    onToggleOutline: () => void
    saving: boolean
    /** Present only for formats whose preview is print-shaped (Fountain). */
    onPrint?: () => void
    onOpenExternal?: () => void
    onReveal?: () => void
    onClose: () => void
  } = $props()

  const layouts: { id: DocumentLayout; label: string; icon: typeof Pencil }[] = [
    { id: 'edit', label: t('document.layout_edit'), icon: Pencil },
    { id: 'split', label: t('document.layout_split'), icon: Columns2 },
    { id: 'preview', label: t('document.layout_preview'), icon: Eye },
  ]
</script>

<header class="toolbar">
  {#if canOutline}
    <button class="icon-btn" class:on={outline} onclick={onToggleOutline} title={t('document.outline')} aria-label={t('document.outline')} aria-pressed={outline}><ListTree size={15} /></button>
  {/if}
  <span class="path" title={path}>{path}</span>
  <SaveIndicator {saving} />
  <div class="segmented" role="group" aria-label={t('document.layout')}>
    {#each layouts as l (l.id)}
      <button class="seg" class:on={layout === l.id} onclick={() => onLayout(l.id)} title={l.label} aria-label={l.label} aria-pressed={layout === l.id}>
        <l.icon size={14} />
        <span class="seg-label">{l.label}</span>
      </button>
    {/each}
  </div>
  {#if onPrint}
    <button class="icon-btn" onclick={onPrint} title={t('document.print')} aria-label={t('document.print')}><Printer size={15} /></button>
  {/if}
  {#if onOpenExternal}
    <button class="icon-btn" onclick={onOpenExternal} title={t('workspace.open_external')} aria-label={t('workspace.open_external')}><ExternalLink size={15} /></button>
  {/if}
  {#if onReveal}
    <button class="icon-btn" onclick={onReveal} title={t('workspace.reveal')} aria-label={t('workspace.reveal')}><FolderSearch size={15} /></button>
  {/if}
  <button class="icon-btn" onclick={onClose} title={t('common.close')} aria-label={t('common.close')}><X size={16} /></button>
</header>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.45rem 0.75rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-elevated);
  }
  .path {
    flex: 1;
    min-width: 0;
    font-size: 0.8rem;
    color: var(--text-muted);
    font-family: "Fira Code", "Consolas", monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .segmented {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    margin: 0 0.35rem;
  }
  .seg {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.3rem 0.6rem;
    border: none;
    background: var(--bg-base);
    color: var(--text-muted);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
  }
  .seg + .seg { border-left: 1px solid var(--border); }
  .seg:hover, .seg:focus-visible { color: var(--text-primary); background: var(--bg-subtle-hover); outline: none; }
  .seg.on { color: var(--text-primary); background: var(--accent-glow-2); }
  .icon-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: 6px;
    display: flex;
  }
  .icon-btn:hover, .icon-btn:focus-visible { color: var(--text-primary); background: var(--bg-subtle-hover); outline: none; }
  .icon-btn.on { color: var(--accent-light); }
  @media (max-width: 900px) {
    .seg-label { display: none; }
  }
</style>
