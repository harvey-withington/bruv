<script lang="ts">
  import { onMount } from 'svelte'
  import { apiFetch, saveActiveRepoID } from '../lib/auth'
  import { resetBrowseCache } from '../lib/browse.svelte'
  import { loadRepoMeta, resetRepoMeta } from '../lib/repoMeta.svelte'
  import { replace } from '../lib/router.svelte'
  import { t } from '../lib/i18n.svelte'

  // Server-side RepoSummary shape (transport/http/server.go).
  type RepoSummary = {
    id: string
    name: string
    disabled?: boolean
  }

  let repos = $state<RepoSummary[]>([])
  let loading = $state(true)
  let errorMsg = $state<string | null>(null)

  onMount(async () => {
    try {
      const res = await apiFetch('/repos')
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
      repos = (await res.json()) as RepoSummary[]
      // If exactly one repo is enabled, auto-select it. The picker
      // exists for the multi-repo case; the single-repo home server is
      // probably the common shape and shouldn't make the user tap.
      const enabled = repos.filter((r) => !r.disabled)
      if (enabled.length === 1) {
        select(enabled[0])
      }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : t('repo_picker.err_load')
    } finally {
      loading = false
    }
  })

  function select(repo: RepoSummary) {
    // Drop any browse-tree + per-repo metadata from the previously-
    // selected repo so the user doesn't briefly see another vault's
    // brands or stale colours while the new ones load.
    resetBrowseCache()
    resetRepoMeta()
    saveActiveRepoID(repo.id)
    // Pre-warm card types + global tag colour map for the newly-
    // active repo. Per-project tags load lazily on first project /
    // card view.
    loadRepoMeta()
    replace('/')
  }
</script>

<main>
  <header>
    <h1>{t('repo_picker.title')}</h1>
    <p class="subtitle">{t('repo_picker.subtitle')}</p>
  </header>

  {#if loading}
    <p class="status">{t('repo_picker.loading')}</p>
  {:else if errorMsg}
    <div class="error" role="alert">
      <p class="error-text">{errorMsg}</p>
      <button type="button" class="retry" onclick={() => window.location.reload()}>
        {t('common.retry')}
      </button>
    </div>
  {:else if repos.length === 0}
    <div class="empty">
      <h2>{t('repo_picker.empty_title')}</h2>
      <p>{t('repo_picker.empty_body')}</p>
    </div>
  {:else}
    <ul class="repo-list">
      {#each repos as repo (repo.id)}
        <li>
          <button
            type="button"
            class="repo-button"
            class:disabled={repo.disabled}
            onclick={() => !repo.disabled && select(repo)}
            disabled={repo.disabled}
          >
            <span class="repo-name">{repo.name}</span>
            {#if repo.disabled}
              <span class="repo-badge">{t('repo_picker.disabled_label')}</span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  main {
    min-height: 100vh;
    padding: 2rem 1.25rem;
    max-width: 480px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
  }

  header {
    margin-bottom: 1.5rem;
  }

  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.5rem;
    color: var(--text);
  }

  .subtitle {
    margin: 0;
    color: var(--text-muted);
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .status,
  .empty p {
    color: var(--text-muted);
    text-align: center;
    margin: 2rem 0;
    font-size: 0.95rem;
  }

  .empty {
    text-align: center;
    margin-top: 2rem;
  }

  .empty h2 {
    margin: 0 0 0.5rem;
    font-size: 1.1rem;
    color: var(--text);
  }

  .error {
    margin-top: 1rem;
    padding: 1rem;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.4);
    border-radius: 8px;
    text-align: center;
  }

  .error-text {
    margin: 0 0 0.75rem;
    color: #fca5a5;
    font-size: 0.9rem;
  }

  .retry {
    background: var(--accent);
    color: var(--bg);
    border: none;
    border-radius: 6px;
    padding: 0.5rem 1.25rem;
    font: inherit;
    font-weight: 600;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .repo-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .repo-button {
    width: 100%;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 1rem 1.1rem;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
    cursor: pointer;
    text-align: left;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    transition: border-color 120ms ease, background 120ms ease;
  }

  .repo-button:not(.disabled):hover,
  .repo-button:not(.disabled):focus-visible {
    border-color: var(--accent);
    outline: none;
  }

  .repo-button:not(.disabled):active {
    transform: scale(0.99);
  }

  .repo-button.disabled {
    color: var(--text-faint);
    cursor: not-allowed;
  }

  .repo-name {
    font-weight: 500;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-badge {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
    background: var(--bg);
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    flex-shrink: 0;
  }
</style>
