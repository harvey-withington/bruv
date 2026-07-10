<script lang="ts">
  // Unified Repo Selector — the single surface for picking what
  // backend + repo to work on. Folds together what used to be two
  // components (the modal "Connections" dialog and the fullscreen
  // "Connection-tree picker"):
  //
  //   - As a fullscreen welcome screen when no repo is open.
  //   - As a modal popped from the sidebar / status chip after a
  //     repo is open and the user wants to switch.
  //
  // The data + actions are identical in both modes; only the
  // chrome (title bar, close button, full-screen background) differs.
  //
  // Capabilities:
  //   - View every known connection (Local + each Remote) and its
  //     repos in one tree.
  //   - Click a repo to switch to it (reload).
  //   - Add a Remote ("Add a server…") via the existing AddConnectionForm.
  //   - Edit an existing Remote (name / URL / token) — new.
  //   - Remove a Remote.
  //   - Per-repo enable/disable toggle on each row.
  //   - For Local: + button opens the OS folder picker, then a small
  //     name-and-confirm step. Existing-vs-new is detected from the
  //     picked folder (manifest.json present? open it. otherwise init).

  import { fade } from 'svelte/transition'
  import {
    X, Server, Monitor, Pause, Play, Plus, Check,
    Trash2, Pencil, AlertTriangle, ChevronDown, ChevronRight,
  } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { focusTrap, focusOnMount, inlineEdit } from '../lib/actions'
  import { showConfirm } from '../lib/confirm.svelte'
  import { showToast } from '../lib/toast.svelte'
  import BruvIcon from './BruvIcon.svelte'
  import AddConnectionForm from './AddConnectionForm.svelte'
  import {
    connections, removeConnection, addConnection, updateConnection,
  } from '../lib/connections.svelte'
  import {
    listAllConnectionRepos, setRepoEnabled, switchToRepo,
    inspectRepoOnConnection, addRepoOnConnection,
    renameRepoOnConnection, removeRepoOnConnection,
    type TreeConnectionNode,
  } from '../lib/repos.svelte'
  import { PickFolder, GetCurrentRepo } from '@shared/api'
  import { nav } from '../lib/store.svelte'

  let {
    mode = 'fullscreen',
    onClose,
  }: {
    mode?: 'fullscreen' | 'dialog'
    onClose?: () => void
  } = $props()

  // ----- state -----

  type View = 'tree' | 'add-server' | 'edit-server' | 'configure-local'
  let view = $state<View>('tree')

  let nodes = $state<TreeConnectionNode[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let collapsed = $state<Record<string, boolean>>({})

  // Edit-server form state
  let editingID = $state<string | null>(null)
  let editName = $state('')
  let editURL = $state('')
  let editToken = $state('') // empty = leave token unchanged

  // Configure-repo state — populated after the user picks a folder
  // via the OS picker. `existing` decides whether the Name field is
  // a read-only display (manifest already names the repo) or a
  // required input (fresh folder needs naming before init).
  // pendingConnectionID is the target connection the new repo gets
  // registered against — uniform across Local + Remote (today's
  // picker only exposes the + button on Local since the OS folder
  // picker can only see the local filesystem; the field is here so
  // the deferred remote folder picker can drop in without a rewrite).
  let pendingConnectionID = $state('')
  let pendingPath = $state('')
  let pendingExisting = $state(false)
  let nameInput = $state('')

  // Inline-rename state for Local repo rows. Keyed by repo ID so that
  // a refresh of `nodes` (which rebuilds row identities) doesn't
  // accidentally apply a draft to the wrong row.
  let renamingID = $state<string | null>(null)
  let renameDraft = $state('')

  $effect(() => { void load() })

  async function load() {
    loading = true
    error = null
    try {
      nodes = await listAllConnectionRepos()
    } catch (e) {
      error = (e as Error)?.message ?? String(e)
    } finally {
      loading = false
    }
  }

  function toggleCollapsed(connectionID: string) {
    collapsed[connectionID] = !collapsed[connectionID]
  }

  // ----- actions -----

  async function pickRepo(connectionID: string, repoID: string, disabled: boolean) {
    if (disabled) {
      showToast(t('tree.row_disabled_hint'), 'info')
      return
    }
    try {
      // switchToRepo handles both cases: same-connection
      // (delegates to selectRepo) and cross-connection (pre-sets
      // the target's last-active-repo via the Shell binding so the
      // post-switch reload actually OPENS the chosen repo instead
      // of dumping the user back on the picker).
      await switchToRepo(connectionID, repoID)
    } catch (e) {
      console.error('Failed to switch repo:', e)
      showToast(t('connection.switch_failed'), 'error')
    }
  }

  async function toggleEnabled(connectionID: string, repoID: string, currentlyDisabled: boolean) {
    // Every connection (Local + Remote) now exposes its own URL +
    // token in connections.json, so toggle hits the right backend
    // directly via per-connection HTTP — no need to be on that
    // connection first.
    try {
      await setRepoEnabled(connectionID, repoID, currentlyDisabled)
      await load()
    } catch (e) {
      console.error('Failed to toggle repo enabled state:', e)
      showToast(t('selector.err_toggle_repo'), 'error')
    }
  }

  function startEdit(connectionID: string) {
    const c = connections.connections.find(x => x.id === connectionID)
    if (!c) return
    editingID = connectionID
    editName = c.name
    editURL = c.url
    editToken = ''
    view = 'edit-server'
  }

  async function saveEdit() {
    if (!editingID) return
    try {
      await updateConnection(editingID, editName.trim(), editURL.trim(), editToken.trim())
      editingID = null
      view = 'tree'
      await load()
      showToast(t('connection.updated'), 'success')
    } catch (e) {
      console.error('Failed to update connection:', e)
      showToast(t('connection.edit_failed'), 'error')
    }
  }

  async function removeRemote(connectionID: string, name: string) {
    const ok = await showConfirm(t('connection.remove_confirm', { name }))
    if (!ok) return
    try {
      await removeConnection(connectionID)
      await load()
      showToast(t('connection.removed'), 'success')
    } catch (e) {
      console.error('Failed to remove connection:', e)
      showToast(t('connection.remove_failed'), 'error')
    }
  }

  async function handleAddEnrolled(args: { name: string; url: string; deviceToken: string }) {
    try {
      await addConnection(args.name, args.url, args.deviceToken, { activate: true })
      // activate: true → reloads.
    } catch (e) {
      console.error('Failed to add connection:', e)
      showToast(t('connection.add_failed'), 'error')
    }
  }

  // beginRenameRepo enters the inline-edit state for a repo row,
  // seeding the draft with the current name so the user can tweak
  // rather than retype. Uniform across Local + Remote.
  function beginRenameRepo(repoID: string, currentName: string) {
    renamingID = repoID
    renameDraft = currentName
  }

  function cancelRenameRepo() {
    renamingID = null
    renameDraft = ''
  }

  // commitRenameRepo persists the new name on the given connection
  // (registry + in-repo manifest are written atomically server-side).
  // After success we refresh nodes so the picker shows the new name,
  // and re-fetch GetCurrentRepo when the renamed repo is the active
  // one so the chip label stays in sync without a reload.
  async function commitRenameRepo(connectionID: string, repoID: string) {
    const name = renameDraft.trim()
    if (!name) {
      showToast(t('welcome.name_required'), 'error')
      return
    }
    try {
      await renameRepoOnConnection(connectionID, repoID, name)
      renamingID = null
      renameDraft = ''
      await load()
      try {
        const current = await GetCurrentRepo()
        if (current?.name) {
          nav.repoName = current.name
        }
      } catch { /* chip just stays stale until next boot — non-fatal */ }
      showToast(t('selector.rename_done'), 'success')
    } catch (e) {
      console.error('Failed to rename repo:', e)
      showToast(t('selector.err_rename_repo'), 'error')
    }
  }

  // removeRepoEntry drops a repo from the given connection's registry.
  // The folder on disk is left alone — this is purely a "stop showing
  // this in the picker" action.
  async function removeRepoEntry(e: MouseEvent, connectionID: string, repoID: string, name: string) {
    e.stopPropagation()
    const ok = await showConfirm(t('tree.remove_local_confirm', { name }))
    if (!ok) return
    // Capture "was this the active one" BEFORE the remove — backend
    // unloads the runtime, so a post-remove GetCurrentRepo against
    // the same repoID would 404 and we'd miss the cue to reload.
    let wasActive = false
    try {
      const current = await GetCurrentRepo()
      wasActive = !!current && current.id === repoID
    } catch { /* non-fatal */ }
    try {
      await removeRepoOnConnection(connectionID, repoID)
      if (wasActive) {
        setTimeout(() => window.location.reload(), 50)
        return
      }
      await load()
    } catch (err) {
      console.error('Failed to remove repo entry:', err)
      showToast(t('selector.err_remove_repo'), 'error')
    }
  }

  // pickAndConfigureRepo unifies "add new" + "add existing" — the OS
  // picker already lets the user create a folder via its built-in
  // "New Folder" button, so we just inspect the picked path and route
  // accordingly. Goes via PickFolder (Wails Shell — picks a folder on
  // THIS machine) regardless of which connection we're adding to;
  // adding to a remote will need a remote folder picker, deferred.
  async function pickAndConfigureRepo(connectionID: string) {
    let path: string
    try {
      path = await PickFolder(t('welcome.pick_folder'))
    } catch {
      return // user cancelled
    }
    if (!path) return
    try {
      const info = await inspectRepoOnConnection(connectionID, path)
      pendingConnectionID = connectionID
      pendingPath = path
      pendingExisting = info.exists
      nameInput = info.exists ? (info.name ?? '') : folderBasename(path)
      error = null
      view = 'configure-local'
    } catch (e) {
      console.error('Failed to inspect folder:', e)
      showToast(t('selector.err_inspect_repo'), 'error')
    }
  }

  function folderBasename(path: string): string {
    // Take the trailing path segment in either separator style.
    const trimmed = path.replace(/[\\/]+$/, '')
    const idx = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'))
    return idx >= 0 ? trimmed.slice(idx + 1) : trimmed
  }

  async function submitConfigure() {
    const name = nameInput.trim()
    if (!pendingExisting && !name) {
      error = t('welcome.name_required')
      return
    }
    try {
      const summary = await addRepoOnConnection(pendingConnectionID, pendingPath, name)
      // switchToRepo handles same-connection (selectRepo + reload)
      // and cross-connection (Shell pre-set + switchConnection +
      // reload). Either path tears down stale state cleanly.
      await switchToRepo(pendingConnectionID, summary.id)
    } catch (e) {
      error = (e as Error)?.message ?? String(e)
    }
  }

  // ----- shell handlers -----

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (view !== 'tree') {
        view = 'tree'
        editingID = null
        error = null
        return
      }
      onClose?.()
    }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (mode !== 'dialog') return
    if (e.target === e.currentTarget) onClose?.()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div
  class="shell"
  class:fullscreen={mode === 'fullscreen'}
  class:dialog={mode === 'dialog'}
  role="presentation"
  onclick={handleOverlayClick}
  out:fade={{ duration: 150 }}
>
  <div class="card" use:focusTrap>
    {#if mode === 'dialog'}
      <div class="header">
        <h2>{t('selector.title')}</h2>
        <button class="close-btn" onclick={() => onClose?.()} title={t('common.close')}><X size={18} /></button>
      </div>
    {:else}
      <div class="welcome-header">
        <BruvIcon size={64} />
        <h1>{t('app.name')}</h1>
      </div>
    {/if}

    <div class="body">
      {#if view === 'add-server'}
        <p class="subtitle">{t('connection.add_subtitle')}</p>
        <AddConnectionForm
          onEnrolled={handleAddEnrolled}
          onCancel={() => { view = 'tree'; error = null }}
          submitLabel={t('connection.add_and_switch')}
        />
      {:else if view === 'edit-server'}
        <p class="subtitle">{t('selector.edit_subtitle')}</p>
        <div class="form">
          <label>
            <span>{t('connection.field_name')}</span>
            <input type="text" bind:value={editName} />
          </label>
          <label>
            <span>{t('connection.field_url')}</span>
            <input type="text" bind:value={editURL} />
          </label>
          <label>
            <span>{t('selector.replace_token')}</span>
            <input type="password" bind:value={editToken} placeholder={t('selector.replace_token_placeholder')} autocomplete="off" />
            <small>{t('selector.replace_token_hint')}</small>
          </label>
          {#if error}<div class="error">{error}</div>{/if}
          <div class="form-actions">
            <button class="btn-secondary" onclick={() => { view = 'tree'; editingID = null; error = null }}>{t('common.cancel')}</button>
            <button class="btn-primary" onclick={saveEdit}>{t('common.save')}</button>
          </div>
        </div>
      {:else if view === 'configure-local'}
        <p class="subtitle">
          {pendingExisting ? t('welcome.configure_subtitle_existing') : t('welcome.configure_subtitle_new')}
        </p>
        <div class="form">
          <label>
            <span>{t('welcome.repo_path_label')}</span>
            <input type="text" value={pendingPath} readonly class="readonly" />
          </label>
          <label>
            <span>{t('welcome.repo_name')}</span>
            {#if pendingExisting}
              <input type="text" value={nameInput} readonly class="readonly" />
            {:else}
              <input type="text" bind:value={nameInput} placeholder={t('welcome.repo_name_placeholder')} use:focusOnMount={true} />
            {/if}
          </label>
          {#if error}<div class="error">{error}</div>{/if}
          <div class="form-actions">
            <button class="btn-secondary" onclick={() => { view = 'tree'; error = null }}>{t('common.cancel')}</button>
            <button class="btn-primary" onclick={submitConfigure}>
              {pendingExisting ? t('welcome.open_repo') : t('welcome.create_repo')}
            </button>
          </div>
        </div>
      {:else}
        {#if loading}
          <p class="muted">{t('common.loading')}</p>
        {:else if error}
          <div class="error-block"><AlertTriangle size={14} /> <span>{error}</span></div>
          <button class="btn-secondary" onclick={load}>{t('common.retry')}</button>
        {:else}
          <div class="tree">
            {#each nodes as node (node.connectionID)}
              <div class="conn-block" class:active={node.connectionID === connections.active}>
                <div class="conn-header">
                  <button
                    class="icon-btn"
                    onclick={() => toggleCollapsed(node.connectionID)}
                    title={collapsed[node.connectionID] ? t('tree.expand') : t('tree.collapse')}
                  >
                    {#if collapsed[node.connectionID]}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
                  </button>
                  <span class="conn-icon">
                    {#if node.isLocal}<Monitor size={14} />{:else}<Server size={14} />{/if}
                  </span>
                  <span class="conn-name">{node.connectionName}</span>

                  {#if !node.reachable && !node.isLocal}
                    <span class="conn-status unreachable" title={node.error ?? ''}>
                      <AlertTriangle size={11} /> {t('tree.unreachable')}
                    </span>
                  {:else if node.repos.length > 0}
                    <span class="conn-status">{node.repos.length}</span>
                  {/if}

                  <!-- Per-connection actions. + button is gated on
                       Local because the OS PickFolder can only see
                       this machine's filesystem; remote folder picker
                       is deferred. Edit / remove only on Remotes —
                       Local is built-in and can't be removed. -->
                  {#if node.isLocal}
                    <button
                      class="icon-btn"
                      onclick={(e) => { e.stopPropagation(); pickAndConfigureRepo(node.connectionID) }}
                      title={t('welcome.add_repo')}
                      aria-label={t('welcome.add_repo')}
                    >
                      <Plus size={12} />
                    </button>
                  {:else}
                    <button class="icon-btn" onclick={() => startEdit(node.connectionID)} title={t('selector.edit_connection')}>
                      <Pencil size={12} />
                    </button>
                    <button class="icon-btn danger" onclick={() => removeRemote(node.connectionID, node.connectionName)} title={t('connection.remove')}>
                      <Trash2 size={12} />
                    </button>
                  {/if}
                </div>

                {#if !collapsed[node.connectionID]}
                  <div class="repos">
                    {#each node.repos as repo (repo.id)}
                      <div class="repo-row" class:disabled={repo.disabled}>
                        {#if renamingID === repo.id}
                          <span class="repo-dot" class:on={!repo.disabled} class:off={repo.disabled}></span>
                          <input
                            class="rename-input"
                            type="text"
                            bind:value={renameDraft}
                            use:inlineEdit={{ onCommit: () => commitRenameRepo(node.connectionID, repo.id), onCancel: cancelRenameRepo }}
                            use:focusOnMount={true}
                          />
                          <button
                            class="icon-btn"
                            onclick={() => commitRenameRepo(node.connectionID, repo.id)}
                            title={t('common.save')}
                          >
                            <Check size={11} />
                          </button>
                          <button
                            class="icon-btn"
                            onclick={cancelRenameRepo}
                            title={t('common.cancel')}
                          >
                            <X size={11} />
                          </button>
                        {:else}
                          <button class="repo-pick" onclick={() => pickRepo(node.connectionID, repo.id, !!repo.disabled)}>
                            <span class="repo-dot" class:on={!repo.disabled} class:off={repo.disabled}></span>
                            <span class="repo-name">{repo.name}</span>
                            {#if repo.disabled}<span class="tag">{t('tree.disabled')}</span>{/if}
                          </button>
                          <button
                            class="icon-btn"
                            onclick={() => toggleEnabled(node.connectionID, repo.id, !!repo.disabled)}
                            title={repo.disabled ? t('tree.enable') : t('tree.disable')}
                          >
                            {#if repo.disabled}<Play size={11} />{:else}<Pause size={11} />{/if}
                          </button>
                          <button
                            class="icon-btn"
                            onclick={() => beginRenameRepo(repo.id, repo.name)}
                            title={t('tree.rename_local')}
                          >
                            <Pencil size={11} />
                          </button>
                          <button
                            class="icon-btn danger"
                            onclick={(e) => removeRepoEntry(e, node.connectionID, repo.id, repo.name)}
                            title={t('tree.remove_local')}
                          >
                            <X size={11} />
                          </button>
                        {/if}
                      </div>
                    {/each}

                    {#if node.repos.length === 0 && (node.reachable || node.isLocal)}
                      <p class="empty-row">
                        {#if node.isLocal}
                          {t('welcome.no_local_repos')}
                        {:else}
                          {t('welcome.server_no_repos')}
                        {/if}
                      </p>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}

            <button class="add-server-btn" onclick={() => { view = 'add-server'; error = null }}>
              <Plus size={14} />
              {t('connection.add_action')}
            </button>
          </div>
        {/if}
      {/if}
    </div>
  </div>
</div>

<style>
  .shell {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    z-index: 1000;
  }
  .shell.fullscreen { background: var(--bg-base); }
  .shell.dialog { background: rgba(0, 0, 0, 0.55); }

  .card {
    width: 100%;
    max-width: 540px;
    background: var(--bg-surface, var(--bg-base));
    border: 1px solid var(--border);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    max-height: 90vh;
    overflow: hidden;
  }
  .shell.fullscreen .card {
    background: transparent;
    border: none;
    max-height: none;
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.85rem 1.1rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .header h2 { margin: 0; font-size: 1rem; color: var(--text-strong); }
  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
  }
  .close-btn:hover { color: var(--text-strong); background: var(--bg-elevated); }

  .welcome-header {
    text-align: center;
    margin: 1.5rem 0 1.25rem;
  }
  .welcome-header h1 {
    margin: 0.5rem 0 0;
    font-size: 1.4rem;
    color: var(--text-strong);
  }

  .body {
    padding: 1rem 1.1rem 1.25rem;
    overflow-y: auto;
  }

  .subtitle {
    margin: 0 0 0.85rem;
    color: var(--text-secondary);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .tree { display: flex; flex-direction: column; gap: 0.5rem; }
  .conn-block { background: var(--bg-elevated); border: 1px solid var(--border); border-radius: 8px; overflow: visible; position: relative; }
  .conn-block.active { border-color: color-mix(in srgb, var(--accent) 25%, var(--border)); }

  .conn-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.45rem 0.6rem;
  }

  .conn-icon { color: var(--text-muted); display: inline-flex; }
  .conn-name {
    flex: 1;
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-strong);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conn-status {
    font-size: 0.65rem;
    color: var(--text-muted);
    padding: 0.1rem 0.45rem;
    background: var(--bg-base);
    border-radius: 999px;
  }
  .conn-status.unreachable {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    color: var(--danger-light, var(--danger));
    background: color-mix(in srgb, var(--danger) 12%, transparent);
  }

  .icon-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 3px;
    display: inline-flex;
    align-items: center;
  }
  .icon-btn:hover { color: var(--text-strong); background: var(--bg-base); }
  .icon-btn.danger:hover { color: var(--danger-light, var(--danger)); }

  .repos { display: flex; flex-direction: column; padding: 0 0.4rem 0.5rem; }
  .repo-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.4rem;
    border-radius: 4px;
  }
  .repo-row:hover { background: var(--bg-base); }
  .repo-row.disabled .repo-name { color: var(--text-muted); }

  .repo-pick {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: none;
    border: none;
    cursor: pointer;
    color: inherit;
    font: inherit;
    text-align: left;
    padding: 0.15rem 0;
    min-width: 0;
  }
  .repo-name { font-size: 0.85rem; color: var(--text-strong); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .rename-input {
    flex: 1;
    min-width: 0;
    padding: 0.2rem 0.4rem;
    background: var(--bg-base);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-strong);
    font: inherit;
    font-size: 0.85rem;
  }
  .rename-input:focus { outline: none; box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .tag {
    font-size: 0.6rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    padding: 0.05rem 0.35rem;
    border-radius: 999px;
    background: var(--bg-base);
    color: var(--text-muted);
  }
  .repo-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: var(--text-muted); }
  .repo-dot.on { background: var(--accent); }
  .repo-dot.off { background: var(--border); }

  .empty-row { color: var(--text-muted); font-size: 0.75rem; padding: 0.4rem; margin: 0; }

  .add-server-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.55rem 0.95rem;
    background: transparent;
    color: var(--accent);
    border: 1px dashed var(--accent);
    border-radius: 8px;
    cursor: pointer;
    font: inherit;
    font-weight: 500;
    margin-top: 0.5rem;
    align-self: flex-start;
  }
  .add-server-btn:hover { background: color-mix(in srgb, var(--accent) 8%, transparent); }

  .muted { color: var(--text-muted); font-size: 0.8rem; text-align: center; padding: 1rem; }

  .form { display: flex; flex-direction: column; gap: 0.85rem; }
  .form label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.8rem; }
  .form label span { font-weight: 500; color: var(--text-strong); }
  .form input { padding: 0.5rem 0.65rem; background: var(--bg-base); border: 1px solid var(--border); border-radius: 4px; color: var(--text-strong); font: inherit; }
  .form input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .form input.readonly { background: var(--bg-elevated); color: var(--text-secondary); cursor: default; }
  .form input.readonly:focus { box-shadow: none; border-color: var(--border); }
  .form small { color: var(--text-muted); font-size: 0.7rem; line-height: 1.4; }
  .form-actions { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.25rem; }

  .error { padding: 0.6rem 0.75rem; background: color-mix(in srgb, var(--danger) 15%, transparent); color: var(--danger-light, var(--danger)); border: 1px solid var(--danger); border-radius: 4px; font-size: 0.8rem; }
  .error-block { display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.5rem 0.75rem; background: color-mix(in srgb, var(--danger) 12%, transparent); color: var(--danger-light, var(--danger)); border-radius: 6px; font-size: 0.8rem; margin-bottom: 0.5rem; }

  .btn-primary, .btn-secondary { padding: 0.5rem 0.95rem; border-radius: 6px; font: inherit; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
  .btn-primary { background: var(--accent); color: #fff; }
  .btn-primary:hover { background: var(--accent-hover, var(--accent)); }
  .btn-secondary { background: transparent; color: var(--text-secondary); border-color: var(--border); }
  .btn-secondary:hover { color: var(--text-strong); background: var(--bg-elevated); }
</style>
