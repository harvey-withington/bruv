<script lang="ts">
  // Slide Deck block — read-only on mobile (v1). Authoring a deck is a
  // big-canvas activity that lives on desktop; here we just list the slides
  // so the deck is visible/reviewable. Presenter remote + swipe playback are
  // the planned mobile follow-ups.

  import { t } from '../../lib/i18n.svelte'
  import type { Block, Slide } from '@shared/types'
  import { resolveContentType } from '@shared/slideContentTypes'
  import { asSlideDeck } from './narrow'

  let { block }: { block: Block } = $props()

  const deck = $derived(asSlideDeck(block.value))

  // First non-empty field value, else the content-type name, else Untitled.
  function slideLabel(slide: Slide): string {
    const ct = resolveContentType(slide.contentTypeId)
    if (ct) {
      for (const f of ct.fields) {
        const v = slide.values?.[f.key]
        if (v && v.trim()) return v.trim()
      }
      return t('slide.ct.' + slide.contentTypeId)
    }
    return t('slide.untitled')
  }

  function contentTypeName(slide: Slide): string {
    return resolveContentType(slide.contentTypeId) ? t('slide.ct.' + slide.contentTypeId) : slide.contentTypeId
  }
</script>

{#if deck.slides.length === 0}
  <p class="empty">{t('slide.empty_mobile')}</p>
{:else}
  <ul class="slides">
    {#each deck.slides as slide (slide.id)}
      <li class="slide">
        {#if slide.thumbnail}
          <img class="thumb" src={slide.thumbnail} alt="" />
        {:else}
          <span class="thumb kind-label">{contentTypeName(slide)}</span>
        {/if}
        <span class="title">{slideLabel(slide)}</span>
        {#if slide.durationSec}<span class="dur">{slide.durationSec}s</span>{/if}
      </li>
    {/each}
  </ul>
  <p class="hint">{t('slide.mobile_readonly')}</p>
{/if}

<style>
  .empty {
    color: var(--text-muted);
    font-size: 0.9rem;
    margin: 0;
  }
  .slides {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }
  .slide {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.4rem 0.5rem;
    background: var(--bg-elevated, var(--bg));
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .thumb {
    width: 2.4rem;
    height: 2.4rem;
    flex-shrink: 0;
    border-radius: 6px;
    object-fit: cover;
    background: var(--bg);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .kind-label {
    font-size: 0.6rem;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: var(--text-muted);
    text-align: center;
    line-height: 1.1;
    padding: 0 0.2rem;
  }
  .title {
    flex: 1;
    min-width: 0;
    font-size: 0.95rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dur {
    font-size: 0.75rem;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .hint {
    font-size: 0.75rem;
    color: var(--text-muted);
    font-style: italic;
    margin: 0.5rem 0 0;
  }
</style>
