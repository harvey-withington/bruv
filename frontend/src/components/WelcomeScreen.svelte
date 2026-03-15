<script lang="ts">
  import { nav } from '../lib/store.svelte'
  import { InitRepository, OpenRepository, PickFolder, ListRecentRepos, RemoveRecentRepo } from '../lib/api'

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
    <h1 class="logo">BRUV</h1>
    <p class="tagline">Your most organised mate.</p>

    {#if mode === 'choose'}
      <div class="actions">
        <button class="btn btn-primary" onclick={() => mode = 'init'}>
          Create New Repository
        </button>
        <button class="btn btn-secondary" onclick={() => mode = 'open'}>
          Open Existing Repository
        </button>
      </div>

      {#if recentRepos.length > 0}
        <div class="recent">
          <h3 class="recent-title">Recent Repositories</h3>
          <div class="recent-list">
            {#each recentRepos as repo}
              <!-- svelte-ignore a11y_no_static_element_interactions a11y_click_events_have_key_events -->
              <div class="recent-item" onclick={() => openRecent(repo.path)}>
                <div class="recent-info">
                  <span class="recent-name">{repo.name}</span>
                  <span class="recent-path">{repo.path}</span>
                </div>
                <button class="recent-remove" onclick={(e) => removeRecent(e, repo.path)} title="Remove from recent">✕</button>
              </div>
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
            <button class="btn btn-browse" onclick={() => browseFolder('Choose base folder')}>Browse</button>
          </div>
        </label>
        {#if error}<p class="error">{error}</p>{/if}
        <div class="form-actions">
          <button class="btn btn-primary" onclick={handleInit}>Create</button>
          <button class="btn btn-ghost" onclick={() => { mode = 'choose'; error = '' }}>Back</button>
        </div>
      </div>

    {:else}
      <div class="form">
        <label>
          <span>Repository Path</span>
          <div class="path-row">
            <input type="text" bind:value={repoPath} placeholder="C:\Users\you\my-workspace" />
            <button class="btn btn-browse" onclick={() => browseFolder('Choose repository folder')}>Browse</button>
          </div>
        </label>
        {#if error}<p class="error">{error}</p>{/if}
        <div class="form-actions">
          <button class="btn btn-primary" onclick={handleOpen}>Open</button>
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
    color: #f5f5f5;
  }

  .tagline {
    font-size: 1rem;
    color: #a1a1aa;
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
    color: #a1a1aa;
    font-weight: 500;
  }

  .form input {
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    border: 1px solid #3f3f46;
    background: #27272a;
    color: #f5f5f5;
    font-size: 0.9rem;
    outline: none;
    transition: border-color 0.15s;
  }

  .form input:focus {
    border-color: #6366f1;
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
    background: #3f3f46;
    color: #e4e4e7;
    flex-shrink: 0;
    padding: 0.5rem 0.75rem;
    font-size: 0.8rem;
  }
  .btn-browse:hover {
    background: #52525b;
  }

  .form-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .error {
    color: #f87171;
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
    background: #6366f1;
    color: #fff;
  }
  .btn-primary:hover {
    background: #4f46e5;
  }

  .btn-secondary {
    background: #3f3f46;
    color: #e4e4e7;
  }
  .btn-secondary:hover {
    background: #52525b;
  }

  .btn-ghost {
    background: transparent;
    color: #a1a1aa;
  }
  .btn-ghost:hover {
    color: #e4e4e7;
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
    color: #52525b;
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
    background: #27272a;
    border-color: #3f3f46;
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
    color: #e4e4e7;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .recent-path {
    font-size: 0.7rem;
    color: #52525b;
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
    color: #52525b;
  }

  .recent-remove:hover {
    color: #f87171 !important;
  }
</style>
