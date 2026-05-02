<script lang="ts">
  import { onMount } from 'svelte'
  import { Inbox } from 'lucide-svelte'
  import { browse, loadBrands, loadStreams, loadProjects } from '../lib/browse.svelte'
  import { navigate, projectURL } from '../lib/router.svelte'
  import { readEnrolment, readActiveRepoID, apiFetch } from '../lib/auth'
  import { t } from '../lib/i18n.svelte'
  import type { Brand, Stream } from '../lib/model'
  import DynamicIcon from '../components/DynamicIcon.svelte'

  // Top-level mobile entry: Inbox tile + browsable Brand → Stream →
  // Project tree. Tapping a project navigates to /m/p/<b>/<s>/<p>.
  // The full zoom UI for Project ↔ Category ↔ Card lands in step 10.

  const enrolment = readEnrolment()

  let activeRepoName = $state<string | null>(null)
  // Expansion state lives in the browse store so it survives the
  // BrowsePage being unmounted while the user is inside a project /
  // category / card. Local component state would reset on every
  // remount and force the user to re-expand.
  const expandedBrands = browse.expandedBrands
  const expandedStreams = browse.expandedStreams

  onMount(async () => {
    loadBrands()
    // Cosmetic: show the active vault name in the header. Best-effort;
    // missing/failed lookup doesn't block browsing.
    try {
      const activeID = readActiveRepoID()
      if (!activeID) return
      const res = await apiFetch('/repos')
      if (!res.ok) return
      const repos = (await res.json()) as Array<{ id: string; name: string }>
      activeRepoName = repos.find((r) => r.id === activeID)?.name ?? null
    } catch {
      /* silent — header label is decorative */
    }
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
</script>

<header class="topbar">
  <button type="button" class="vault-button" onclick={() => navigate('/repos')} title={t('browse.switch_vault')}>
    <span class="vault-name">{activeRepoName ?? t('common.loading')}</span>
    <span class="vault-arrow">›</span>
  </button>
</header>

<main>
  <button type="button" class="inbox-tile" onclick={() => navigate('/inbox')}>
    <span class="inbox-icon" aria-hidden="true"><Inbox size={22} /></span>
    <div class="tile-text">
      <span class="tile-title">{t('browse.inbox')}</span>
      <span class="tile-sub">{t('browse.inbox_sub')}</span>
    </div>
  </button>

  <h2 class="section">{t('browse.brands')}</h2>

  {#if browse.brands.state === 'loading'}
    <p class="status">{t('common.loading')}</p>
  {:else if browse.brands.state === 'error'}
    <p class="error">{browse.brands.error}</p>
  {:else if browse.brands.items.length === 0}
    <p class="status">{t('browse.empty')}</p>
  {:else}
    <ul class="tree">
      {#each browse.brands.items as brand (brand.id)}
        <li class="brand">
          <button type="button" class="row brand-row" onclick={() => toggleBrand(brand)}>
            <span class="caret" class:open={expandedBrands[brand.slug]} aria-hidden="true">▸</span>
            {#if brand.icon}
              <DynamicIcon name={brand.icon} size={18} />
            {/if}
            <span class="row-name">{brand.name}</span>
          </button>

          {#if expandedBrands[brand.slug]}
            {@const streams = browse.streamsFor(brand.slug)}
            {#if !streams || streams.state === 'loading'}
              <p class="indent status">{t('common.loading')}</p>
            {:else if streams.state === 'error'}
              <p class="indent error">{streams.error}</p>
            {:else if streams.items.length === 0}
              <p class="indent status">{t('browse.empty_stream')}</p>
            {:else}
              <ul class="streams">
                {#each streams.items as stream (stream.id)}
                  {@const streamKey = `${brand.slug}/${stream.slug}`}
                  <li class="stream">
                    <button type="button" class="row stream-row" onclick={() => toggleStream(brand, stream)}>
                      <span class="caret" class:open={expandedStreams[streamKey]} aria-hidden="true">▸</span>
                      {#if stream.icon}
                        <DynamicIcon name={stream.icon} size={16} />
                      {/if}
                      <span class="row-name">{stream.name}</span>
                    </button>

                    {#if expandedStreams[streamKey]}
                      {@const projects = browse.projectsFor(brand.slug, stream.slug)}
                      {#if !projects || projects.state === 'loading'}
                        <p class="indent status">{t('common.loading')}</p>
                      {:else if projects.state === 'error'}
                        <p class="indent error">{projects.error}</p>
                      {:else if projects.items.length === 0}
                        <p class="indent status">{t('browse.empty_project')}</p>
                      {:else}
                        <ul class="projects">
                          {#each projects.items as project (project.id)}
                            <li>
                              <button
                                type="button"
                                class="row project-row"
                                onclick={() => navigate(projectURL(brand.slug, stream.slug, project.slug))}
                              >
                                {#if project.icon}
                                  <DynamicIcon name={project.icon} size={16} />
                                {/if}
                                <span class="row-name">{project.name}</span>
                                <span class="row-arrow" aria-hidden="true">›</span>
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
</main>

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.85rem 1rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 10;
  }

  .vault-button {
    background: transparent;
    border: none;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.25rem 0.5rem;
    border-radius: 6px;
  }

  .vault-button:hover {
    color: var(--text);
    background: var(--bg-elev-1);
  }

  .vault-name {
    max-width: 60vw;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .vault-arrow {
    color: var(--text-faint);
  }

  main {
    padding: 1rem 0.85rem 4rem;
    max-width: 600px;
    margin: 0 auto;
  }

  .inbox-tile {
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1rem 1.1rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.85rem;
    text-align: left;
    transition: border-color 120ms ease;
  }

  .inbox-tile:hover,
  .inbox-tile:focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .inbox-icon {
    color: var(--accent);
    display: inline-flex;
    align-items: center;
  }

  .tile-text {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
  }

  .tile-title {
    font-weight: 600;
    font-size: 1rem;
  }

  .tile-sub {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin-top: 0.15rem;
  }

  .section {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
    margin: 1.75rem 0.25rem 0.75rem;
  }

  .tree,
  .streams,
  .projects {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .streams,
  .projects {
    margin-left: 0.65rem;
    padding-left: 0.65rem;
    border-left: 1px solid var(--border);
    margin-top: 0.25rem;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.65rem 0.75rem;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    cursor: pointer;
    text-align: left;
  }

  .row:hover,
  .row:focus-visible {
    background: var(--bg-elev-1);
    border-color: var(--border);
    outline: none;
  }

  .row-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-arrow {
    color: var(--text-faint);
    font-size: 1.1rem;
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

  .stream-row {
    font-weight: 500;
    font-size: 0.9rem;
  }

  .project-row {
    font-size: 0.9rem;
  }

  .indent {
    margin: 0.25rem 0 0.5rem 1.4rem;
    font-size: 0.85rem;
  }

  .status {
    color: var(--text-muted);
    margin: 0.5rem 0.25rem;
    font-size: 0.85rem;
  }

  .error {
    color: #fca5a5;
    margin: 0.5rem 0.25rem;
    font-size: 0.85rem;
  }
</style>
