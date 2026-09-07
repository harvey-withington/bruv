<script lang="ts">
  import type { OutlineNode } from '@shared/documentFormats'
  import { t } from '../../../lib/i18n.svelte'

  // Structure pane: headings / scenes / chapters from the format module.
  // Click = jump the editor there. The entry the cursor is under is
  // marked, so the pane doubles as "where am I" in a long document.
  let { nodes, cursorLine, onSelect }: {
    nodes: OutlineNode[]
    cursorLine: number
    onSelect: (line: number) => void
  } = $props()

  const activeId = $derived.by(() => {
    let active: OutlineNode | null = null
    for (const n of nodes) {
      if (n.line <= cursorLine) active = n
      else break
    }
    return active?.id ?? null
  })
</script>

<nav class="outline" aria-label={t('document.outline')}>
  {#if nodes.length === 0}
    <p class="empty">{t('document.outline_empty')}</p>
  {:else}
    <ul>
      {#each nodes as n (n.id)}
        <li>
          <button
            class="node"
            class:active={n.id === activeId}
            style:padding-left="{0.5 + (n.level - 1) * 0.7}rem"
            title={n.description ?? n.title}
            onclick={() => onSelect(n.line)}
          >
            <span class="title">{n.title}</span>
            {#if n.description}<span class="desc">{n.description}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</nav>

<style>
  .outline {
    height: 100%;
    overflow: auto;
    padding: 0.5rem 0.35rem;
    border-right: 1px solid var(--border-muted);
    background: var(--bg-surface);
    font-size: 0.8rem;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .node {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    width: 100%;
    gap: 0.1rem;
    padding: 0.3rem 0.5rem;
    border: none;
    border-radius: 5px;
    background: none;
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
    font: inherit;
  }
  .node:hover, .node:focus-visible {
    color: var(--text-primary);
    background: var(--bg-subtle-hover);
    outline: none;
  }
  .node.active {
    color: var(--text-primary);
    background: var(--accent-glow-3);
  }
  .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .desc {
    font-size: 0.72rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
  .empty {
    margin: 0.5rem;
    color: var(--text-faint);
    font-size: 0.78rem;
  }
</style>
