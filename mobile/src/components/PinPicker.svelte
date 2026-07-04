<script lang="ts">
  import { onMount } from 'svelte'
  import { fly, fade } from 'svelte/transition'
  import { X } from 'lucide-svelte'
  import { browse, loadBrands, loadStreams, loadProjects, loadCategories } from '../lib/browse.svelte'
  import { t } from '../lib/i18n.svelte'
  import { renderInline } from '@shared/markdown'
  import DynamicIcon from './DynamicIcon.svelte'
  import type { Brand, Stream, Project, Category } from '../lib/model'

  // Pin destination picker. Shows the full Brand → Stream → Project →
  // Category tree as expandable rows; tapping a category fires
  // onSelect with everything the caller needs to call PinCard.
  //
  // Reuses the same lazy-loading browse store as BrowsePage so
  // anything the user already expanded there is instant here.

  let {
    onSelect,
    onClose,
  }: {
    onSelect: (sel: {
      brand: Brand
      stream: Stream
      project: Project
      category: Category
    }) => void | Promise<void>
    onClose: () => void
  } = $props()

  let expandedBrands = $state<Record<string, boolean>>({})
  let expandedStreams = $state<Record<string, boolean>>({})
  let expandedProjects = $state<Record<string, boolean>>({})

  onMount(() => {
    loadBrands()
  })

  function toggleBrand(brand: Brand) {
    const open = !expandedBrands[brand.slug]
    expandedBrands[brand.slug] = open
    if (open) loadStreams(brand.slug)
  }

  function toggleStream(brand: Brand, stream: Stream) {
    const key = `${brand.slug}/${stream.slug}`
    const open = !expandedStreams[key]
    expandedStreams[key] = open
    if (open) loadProjects(brand.slug, stream.slug)
  }

  function toggleProject(brand: Brand, stream: Stream, project: Project) {
    const key = `${brand.slug}/${stream.slug}/${project.slug}`
    const open = !expandedProjects[key]
    expandedProjects[key] = open
    if (open) loadCategories(brand.slug, stream.slug, project.slug)
  }

  function onBackdrop(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="backdrop" role="presentation" onclick={onBackdrop} transition:fade={{ duration: 120 }}>
  <div class="sheet" transition:fly={{ y: 400, duration: 220 }} role="dialog" aria-modal="true" aria-labelledby="pin-picker-title">
    <header>
      <h2 id="pin-picker-title">{t('pin_picker.title')}</h2>
      <button type="button" class="close" onclick={onClose} aria-label={t('common.cancel')}>
        <X size={18} />
      </button>
    </header>
    <p class="subtitle">{t('pin_picker.subtitle')}</p>

    <div class="tree">
      {#if browse.brands.state === 'loading'}
        <p class="status">{t('common.loading')}</p>
      {:else if browse.brands.state === 'error'}
        <p class="error">{browse.brands.error}</p>
      {:else if browse.brands.items.length === 0}
        <p class="status">{t('browse.empty')}</p>
      {:else}
        <ul>
          {#each browse.brands.items as brand (brand.id)}
            <li>
              <button type="button" class="row brand-row" onclick={() => toggleBrand(brand)}>
                <span class="caret" class:open={expandedBrands[brand.slug]} aria-hidden="true">▸</span>
                {#if brand.icon}<DynamicIcon name={brand.icon} size={16} />{/if}
                <span class="name">{@html renderInline(brand.name)}</span>
              </button>

              {#if expandedBrands[brand.slug]}
                {@const streams = browse.streamsFor(brand.slug)}
                {#if !streams || streams.state === 'loading'}
                  <p class="status indent">{t('common.loading')}</p>
                {:else if streams.state === 'error'}
                  <p class="error indent">{streams.error}</p>
                {:else}
                  <ul class="indent">
                    {#each streams.items as stream (stream.id)}
                      {@const skey = `${brand.slug}/${stream.slug}`}
                      <li>
                        <button type="button" class="row stream-row" onclick={() => toggleStream(brand, stream)}>
                          <span class="caret" class:open={expandedStreams[skey]} aria-hidden="true">▸</span>
                          {#if stream.icon}<DynamicIcon name={stream.icon} size={14} />{/if}
                          <span class="name">{@html renderInline(stream.name)}</span>
                        </button>

                        {#if expandedStreams[skey]}
                          {@const projects = browse.projectsFor(brand.slug, stream.slug)}
                          {#if !projects || projects.state === 'loading'}
                            <p class="status indent">{t('common.loading')}</p>
                          {:else if projects.state === 'error'}
                            <p class="error indent">{projects.error}</p>
                          {:else}
                            <ul class="indent">
                              {#each projects.items as project (project.id)}
                                {@const pkey = `${brand.slug}/${stream.slug}/${project.slug}`}
                                <li>
                                  <button type="button" class="row project-row" onclick={() => toggleProject(brand, stream, project)}>
                                    <span class="caret" class:open={expandedProjects[pkey]} aria-hidden="true">▸</span>
                                    {#if project.icon}<DynamicIcon name={project.icon} size={14} />{/if}
                                    <span class="name">{@html renderInline(project.name)}</span>
                                  </button>

                                  {#if expandedProjects[pkey]}
                                    {@const cats = browse.categoriesFor(brand.slug, stream.slug, project.slug)}
                                    {#if !cats || cats.state === 'loading'}
                                      <p class="status indent">{t('common.loading')}</p>
                                    {:else if cats.state === 'error'}
                                      <p class="error indent">{cats.error}</p>
                                    {:else if cats.items.length === 0}
                                      <p class="status indent">{t('pin_picker.no_categories')}</p>
                                    {:else}
                                      <ul class="indent">
                                        {#each cats.items as category (category.id)}
                                          <li>
                                            <button
                                              type="button"
                                              class="row category-row"
                                              onclick={() => onSelect({ brand, stream, project, category })}
                                            >
                                              {#if category.icon}<DynamicIcon name={category.icon} size={14} />{/if}
                                              <span class="name">{@html renderInline(category.name)}</span>
                                              <span class="pin-here" aria-hidden="true">{t('pin_picker.pin_here')}</span>
                                            </button>
                                          </li>
                                        {/each}
                                      </ul>
                                    {/if}
                                  {/if}
                                </li>
                              {/each}
                            </ul>
                          {/if}
                        {/if}
                      </li>
                    {/each}
                  </ul>
                {/if}
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
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
    box-shadow: 0 -10px 30px rgba(0, 0, 0, 0.35);
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.25rem;
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

  .subtitle {
    margin: 0 0 0.85rem;
    color: var(--text-muted);
    font-size: 0.85rem;
    line-height: 1.4;
  }

  .tree {
    overflow-y: auto;
    flex: 1;
  }

  ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  ul.indent {
    margin-left: 0.6rem;
    padding-left: 0.6rem;
    border-left: 1px solid var(--border);
    margin-top: 0.2rem;
  }

  .status,
  .error {
    margin: 0.4rem 0.25rem;
    font-size: 0.85rem;
  }
  .status {
    color: var(--text-muted);
  }
  .error {
    color: #fca5a5;
  }
  .status.indent,
  .error.indent {
    margin-left: 1rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    padding: 0.55rem 0.65rem;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    cursor: pointer;
    text-align: left;
  }

  .row:hover,
  .row:focus-visible {
    background: var(--bg);
    border-color: var(--border);
    outline: none;
  }

  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .caret {
    color: var(--text-muted);
    font-size: 0.7rem;
    width: 0.7rem;
    text-align: center;
    transition: transform 120ms ease;
  }
  .caret.open {
    transform: rotate(90deg);
  }

  .brand-row {
    font-weight: 600;
  }

  .category-row {
    color: var(--accent);
  }

  .pin-here {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }

  .category-row:hover .pin-here,
  .category-row:focus-visible .pin-here {
    color: var(--accent);
  }
</style>
