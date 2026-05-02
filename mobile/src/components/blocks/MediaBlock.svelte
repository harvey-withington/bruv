<script lang="ts">
  // Media block — read-only gallery for now. Editing the per-item
  // caption / order / membership is a later polish pass; this just
  // displays whatever the backend stored. Mirrors what desktop does
  // for the simpler render path.

  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asMedia } from './narrow'

  let { block }: { block: Block } = $props()

  const items = $derived(asMedia(block.value))
</script>

{#if items.length === 0}
  <p class="empty">{t('block.media.empty')}</p>
{:else}
  <ul class="gallery">
    {#each items as item (item.id)}
      <li>
        {#if item.mime?.startsWith('video')}
          <video src={item.url} controls preload="metadata" class="media"></video>
        {:else}
          <img src={item.url} alt={item.caption ?? ''} class="media" />
        {/if}
        {#if item.caption}
          <p class="caption">{item.caption}</p>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .gallery {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .media {
    width: 100%;
    max-height: 60vh;
    border-radius: 8px;
    border: 1px solid var(--border);
    display: block;
  }
  .caption {
    margin: 0.25rem 0 0;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .empty {
    color: var(--text-faint);
    font-size: 0.85rem;
    margin: 0;
    font-style: italic;
  }
</style>
