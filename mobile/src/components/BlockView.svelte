<script lang="ts">
  import { renderMarkdown } from '@shared/markdown'
  import { Check, Square, CheckSquare, Star } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import type { Block, BlockMeta, ChecklistItem, ListItem } from '@shared/types'

  let { block }: { block: Block } = $props()

  // Helpers — narrow `block.value` for each type. Block values are
  // a JSON union so TS doesn't know which shape we're holding without
  // a runtime check.
  function asString(v: unknown): string {
    return typeof v === 'string' ? v : ''
  }
  function asNumber(v: unknown): number | null {
    return typeof v === 'number' ? v : null
  }
  function asBool(v: unknown): boolean {
    return v === true
  }
  function asChecklist(v: unknown): ChecklistItem[] {
    return Array.isArray(v) ? (v as ChecklistItem[]) : []
  }
  function asList(v: unknown): ListItem[] {
    return Array.isArray(v) ? (v as ListItem[]) : []
  }
  function asUrl(v: unknown): { url: string; caption?: string } | null {
    if (v && typeof v === 'object' && 'url' in v) return v as { url: string; caption?: string }
    return null
  }

  // Cheap, locale-aware date formatter — falls back to the raw value
  // if parsing fails so we never lose user data on screen.
  function formatDate(raw: string, meta?: BlockMeta): string {
    if (!raw) return ''
    const d = new Date(raw)
    if (Number.isNaN(d.getTime())) return raw
    if (meta?.format === 'date-time') return d.toLocaleString()
    return d.toLocaleDateString()
  }
</script>

<section class="block">
  {#if block.label && block.type !== 'divider'}
    <h3 class="label">{block.label}</h3>
  {/if}

  {#if block.type === 'text'}
    <!-- Markdown is rendered via shared/markdown.ts which uses marked
         + a custom link renderer; safe to inject. -->
    <div class="prose">{@html renderMarkdown(asString(block.value))}</div>

  {:else if block.type === 'checklist'}
    <ul class="checklist">
      {#each asChecklist(block.value) as item (item.id)}
        <li class:done={item.done}>
          <span class="check" aria-hidden="true">
            {#if item.done}
              <CheckSquare size={14} />
            {:else}
              <Square size={14} />
            {/if}
          </span>
          <span class="check-text">{item.text}</span>
        </li>
      {/each}
    </ul>

  {:else if block.type === 'list'}
    <ul class="bullet-list">
      {#each asList(block.value) as item (item.id)}
        <li>{item.text}</li>
      {/each}
    </ul>

  {:else if block.type === 'divider'}
    {#if block.label}
      <div class="divider-with-label">
        <span>{block.label}</span>
      </div>
    {:else}
      <hr class="divider" />
    {/if}

  {:else if block.type === 'url'}
    {@const u = asUrl(block.value)}
    {#if u}
      <a class="url-block" href={u.url} target="_blank" rel="noopener noreferrer">
        {u.caption || u.url}
      </a>
    {/if}

  {:else if block.type === 'date'}
    <p class="value">{formatDate(asString(block.value), block.meta)}</p>

  {:else if block.type === 'number'}
    <p class="value">
      {asNumber(block.value) ?? '—'}{#if block.meta?.suffix}<span class="suffix"> {block.meta.suffix}</span>{/if}
    </p>

  {:else if block.type === 'rating'}
    {@const r = asNumber(block.value) ?? 0}
    {@const max = block.meta?.max ?? 5}
    <div class="rating" aria-label={`Rating ${r} of ${max}`}>
      {#each Array(max) as _, i}
        <Star size={16} class={i < r ? 'star-filled' : 'star-empty'} />
      {/each}
    </div>

  {:else if block.type === 'checkbox'}
    <p class="value">
      <span class="check" aria-hidden="true">
        {#if asBool(block.value)}
          <CheckSquare size={14} />
        {:else}
          <Square size={14} />
        {/if}
      </span>
      {block.label || ''}
    </p>

  {:else if block.type === 'select'}
    <p class="value">{asString(block.value) || '—'}</p>

  {:else}
    <!-- Block types that need richer editing UI (media uploads, image
         display, progress, alarms, surveys, multi-choice groups).
         Show the label and value where we can; full rendering lands
         when those features come to mobile. -->
    <p class="placeholder">
      {t('block.unsupported_on_mobile', { type: block.type })}
    </p>
  {/if}
</section>

<style>
  .block {
    margin-bottom: 1.25rem;
  }

  .label {
    margin: 0 0 0.4rem;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .prose {
    font-size: 0.95rem;
    line-height: 1.55;
    color: var(--text);
  }

  /* Loose styling for marked output — full theming can come later. */
  .prose :global(p) {
    margin: 0 0 0.75rem;
  }
  .prose :global(p:last-child) {
    margin-bottom: 0;
  }
  .prose :global(a) {
    color: var(--accent);
  }
  .prose :global(code) {
    background: var(--bg-elev-1);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.85em;
  }
  .prose :global(pre) {
    background: var(--bg-elev-1);
    padding: 0.65rem;
    border-radius: 6px;
    overflow-x: auto;
  }
  .prose :global(pre code) {
    background: transparent;
    padding: 0;
  }
  .prose :global(blockquote) {
    margin: 0.5rem 0;
    padding-left: 0.75rem;
    border-left: 3px solid var(--border);
    color: var(--text-muted);
  }

  .checklist,
  .bullet-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .bullet-list {
    list-style: disc;
    padding-left: 1.25rem;
  }

  .checklist li {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    font-size: 0.95rem;
  }

  .checklist .done .check-text {
    text-decoration: line-through;
    color: var(--text-muted);
  }

  .check {
    color: var(--text-muted);
    flex-shrink: 0;
    margin-top: 2px;
  }

  .checklist .done .check {
    color: var(--accent);
  }

  .divider {
    border: none;
    border-top: 1px solid var(--border);
    margin: 1rem 0;
  }

  .divider-with-label {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    margin: 1rem 0;
    color: var(--text-muted);
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .divider-with-label::before,
  .divider-with-label::after {
    content: '';
    flex: 1;
    border-top: 1px solid var(--border);
  }

  .url-block {
    display: inline-block;
    padding: 0.5rem 0.75rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--accent);
    font-size: 0.9rem;
    word-break: break-all;
  }

  .value {
    margin: 0;
    font-size: 0.95rem;
    color: var(--text);
  }

  .suffix {
    color: var(--text-muted);
  }

  .rating {
    display: inline-flex;
    gap: 0.15rem;
    color: var(--text-faint);
  }

  .rating :global(.star-filled) {
    color: var(--accent);
    fill: var(--accent);
  }

  .rating :global(.star-empty) {
    color: var(--text-faint);
  }

  .placeholder {
    margin: 0;
    padding: 0.5rem 0.75rem;
    background: var(--bg-elev-1);
    border: 1px dashed var(--border);
    border-radius: 6px;
    color: var(--text-faint);
    font-size: 0.85rem;
    font-style: italic;
  }
</style>
