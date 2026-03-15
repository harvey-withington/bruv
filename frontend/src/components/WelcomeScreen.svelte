<script lang="ts">
  import { nav } from '../lib/store.svelte'
  import { InitRepository, OpenRepository, PickFolder, ListRecentRepos, RemoveRecentRepo } from '../lib/api'
  import { X, FolderPlus, FolderOpen, FolderSearch } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  async function browseFolder(title: string) {
    try {
      const path = await PickFolder(title)
      if (path) repoPath = path
    } catch { /* user cancelled */ }
  }

  type Recent = { path: string; name: string; last_opened: string }

  let repoPath = $state('')
  let repoName = $state('')
  let mode = $state<'choose' | 'init' | 'open'>('choose')
  let error = $state('')
  let recentRepos = $state<Recent[]>([])

  $effect(() => {
    loadRecent()
  })

  async function loadRecent() {
    try {
      recentRepos = (await ListRecentRepos()) || []
    } catch { recentRepos = [] }
  }

  async function openRecent(path: string) {
    try {
      error = ''
      await OpenRepository(path)
      nav.repoOpen = true
      nav.repoPath = path
    } catch (e: any) {
      error = e?.message || String(e)
    }
  }

  async function removeRecent(e: MouseEvent, path: string) {
    e.stopPropagation()
    try {
      await RemoveRecentRepo(path)
      await loadRecent()
    } catch { /* ignore */ }
  }

  async function handleInit() {
    if (!repoPath.trim() || !repoName.trim()) {
      error = 'Please provide both a path and a name.'
      return
    }
    try {
      error = ''
      const actualPath = await InitRepository(repoPath.trim(), repoName.trim())
      nav.repoOpen = true
      nav.repoPath = actualPath
    } catch (e: any) {
      error = e?.message || String(e)
    }
  }

  async function handleOpen() {
    if (!repoPath.trim()) {
      error = 'Please provide a repository path.'
      return
    }
    try {
      error = ''
      await OpenRepository(repoPath.trim())
      nav.repoOpen = true
      nav.repoPath = repoPath.trim()
    } catch (e: any) {
      error = e?.message || String(e)
    }
  }
</script>

<div class="welcome">
  <div class="welcome-card">
    <h1 class="logo">{t('app.name')}</h1>
    <p class="tagline">{t('welcome.subtitle')}</p>

    {#if mode === 'choose'}
      <div class="actions">
        <button class="btn btn-primary" onclick={() => mode = 'init'}>
          <FolderPlus size={16} /> {t('welcome.create_repo')}
        </button>
        <button class="btn btn-secondary" onclick={() => mode = 'open'}>
          <FolderOpen size={16} /> {t('welcome.open_repo')}
        </button>
      </div>

      {#if recentRepos.length > 0}
        <div class="recent">
          <h3 class="recent-title">{t('welcome.recent')}</h3>
          <div class="recent-list">
            {#each recentRepos as repo}
              <button class="recent-item" onclick={() => openRecent(repo.path)}>
                <span class="recent-info">
                  <span class="recent-name">{repo.name}</span>
                  <span class="recent-path">{repo.path}</span>
                </span>
                <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                <span class="recent-remove" role="button" tabindex="-1" onclick={(e) => removeRecent(e, repo.path)} title="Remove from recent"><X size={12} /></span>
              </button>
            {/each}
          </div>
        </div>
      {/if}

    {:else if mode === 'init'}
      <div class="form">
        <label>
          <span>Repository Name</span>
          <input type="text" bind:value={repoName} placeholder="My Workspace" />
        </label>
        <label>
          <span>Base Folder</span>
          <div class="path-row">
            <input type="text" bind:value={repoPath} placeholder="C:\Users\you\repos" />
            <button class="btn btn-browse" onclick={() => browseFolder('Choose base folder')}><FolderSearch size={14} /> {t('welcome.browse')}</button>
          </div>
        </label>
        {#if error}<p class="error">{error}</p>{/if}
        <div class="form-actions">
          <button class="btn btn-primary" onclick={handleInit}>{t('welcome.init_submit')}</button>
          <button class="btn btn-ghost" onclick={() => { mode = 'choose'; error = '' }}>Back</button>
        </div>
      </div>

    {:else}
      <div class="form">
        <label>
          <span>Repository Path</span>
          <div class="path-row">
            <input type="text" bind:value={repoPath} placeholder="C:\Users\you\my-workspace" />
            <button class="btn btn-browse" onclick={() => browseFolder('Choose repository folder')}><FolderSearch size={14} /> {t('welcome.browse')}</button>
          </div>
        </label>
        {#if error}<p class="error">{error}</p>{/if}
        <div class="form-actions">
          <button class="btn btn-primary" onclick={handleOpen}>{t('welcome.open_submit')}</button>
          <button class="btn btn-ghost" onclick={() => { mode = 'choose'; error = '' }}>Back</button>
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .welcome {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    user-select: none;
  }

  .welcome-card {
    text-align: center;
    max-width: 400px;
    width: 100%;
    padding: 2rem;
  }

  .logo {
    font-size: 3.5rem;
    font-weight: 800;
    letter-spacing: 0.15em;
    margin: 0;
    color: var(--text-primary);
  }

  .tagline {
    font-size: 1rem;
    color: var(--text-secondary);
    margin: 0.5rem 0 2rem;
  }

  .actions {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .form {
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .form label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .form label span {
    font-size: 0.8rem;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .form input {
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.9rem;
    outline: none;
    transition: border-color 0.15s;
  }

  .form input:focus {
    border-color: var(--accent);
  }

  .path-row {
    display: flex;
    gap: 0.5rem;
  }

  .path-row input {
    flex: 1;
    min-width: 0;
  }

  .btn-browse {
    background: var(--border);
    color: var(--text-strong);
    flex-shrink: 0;
    padding: 0.5rem 0.75rem;
    font-size: 0.8rem;
  }
  .btn-browse:hover {
    background: var(--border-hover);
  }

  .form-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .error {
    color: var(--danger-light);
    font-size: 0.8rem;
    margin: 0;
  }

  .btn {
    padding: 0.6rem 1.2rem;
    border-radius: 6px;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    border: none;
    transition: background 0.15s, opacity 0.15s;
  }

  .btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .btn-primary:hover {
    background: var(--accent-hover);
  }

  .btn-secondary {
    background: var(--border);
    color: var(--text-strong);
  }
  .btn-secondary:hover {
    background: var(--border-hover);
  }

  .btn-ghost {
    background: transparent;
    color: var(--text-secondary);
  }
  .btn-ghost:hover {
    color: var(--text-strong);
  }

  .recent {
    margin-top: 2rem;
    text-align: left;
  }

  .recent-title {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
    margin: 0 0 0.5rem;
  }

  .recent-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .recent-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    background: none;
    border: 1px solid transparent;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s, border-color 0.1s;
  }

  .recent-item:hover {
    background: var(--bg-elevated);
    border-color: var(--border);
  }

  .recent-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
    overflow: hidden;
  }

  .recent-name {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-strong);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .recent-path {
    font-size: 0.7rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .recent-remove {
    background: none;
    border: none;
    color: transparent;
    cursor: pointer;
    font-size: 0.7rem;
    padding: 0.2rem 0.3rem;
    flex-shrink: 0;
    transition: color 0.1s;
  }

  .recent-item:hover .recent-remove {
    color: var(--text-faint);
  }

  .recent-remove:hover {
    color: var(--danger-light) !important;
  }
</style>
