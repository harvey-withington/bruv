<script lang="ts">
  // "This device's copy of the workspace."
  //
  // On a remote connection the workspace's files live on the server, so
  // Tier 1 actions (open in an editor, reveal in Explorer) have nothing
  // local to act on. This section closes that gap the way the spec
  // intends: the host publishes the folder as git, and this device clones
  // a real working copy onto its own disk.
  //
  // Two phases, both visible: the host prepares (git init + initial commit,
  // reported through Workspace.git_serve so every client sees it), then
  // this device clones (reported by the shell's checkout status).
  import { HardDriveDownload, FolderOpen, ArrowDownToLine, ArrowUpFromLine, Unlink, Loader, AlertTriangle, GitMerge } from 'lucide-svelte'
  import {
    DisableWorkspaceGitServe,
    EnableWorkspaceGitServe,
    ForgetWorkspaceCheckout,
    GetWorkspaceCheckout,
    InspectWorkspaceGitServe,
    MaterializeWorkspace,
    MergeWorkspaceCheckout,
    OpenWorkspacePath,
    PickFolder,
    PullWorkspaceCheckout,
    PushWorkspaceCheckout,
  } from '@shared/api'
  import type { Workspace, WorkspaceCheckoutInfo, WorkspaceSyncResult } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import { showConfirm } from '../../lib/confirm.svelte'
  import { formatBytes } from '../../lib/format'

  let { ws, brandSlug, streamSlug, projectSlug, serverName, onCheckoutChange }: {
    ws: Workspace
    brandSlug: string
    streamSlug: string
    projectSlug: string
    serverName: string
    // Lets the panel point its Tier 1 actions at the local copy instead of
    // the server-side path, and re-render when that changes.
    onCheckoutChange?: (info: WorkspaceCheckoutInfo | null) => void
  } = $props()

  let checkout = $state<WorkspaceCheckoutInfo | null>(null)
  let busy = $state(false)
  // Set when THIS session asked the host to publish, so the clone starts by
  // itself once the host reports ready — the user pressed one button and
  // shouldn't have to press a second one when the server finishes.
  let awaitingPublish = $state(false)

  const publishing = $derived(ws.git_serve === 'initializing')
  const published = $derived(ws.git_serve === 'ready')
  const cloning = $derived(checkout?.status === 'cloning')
  const hasCopy = $derived(checkout?.has_copy === true)

  async function refresh() {
    try {
      // Assign once and hand the caller the same value: reading `checkout`
      // back inside an effect is how a $effect turns into a loop, and a
      // runaway effect halts the whole Svelte runtime.
      const info = await GetWorkspaceCheckout(ws.id)
      checkout = info
      onCheckoutChange?.(info)
    } catch (e) {
      showToast(t('workspace.copy_status_failed', { error: message(e) }), 'error')
    }
  }

  // Poll only while a clone is running: it's the one state with no event to
  // ride, and it ends. Idle panels make no calls at all.
  $effect(() => {
    if (!cloning) return
    const timer = setInterval(refresh, 700)
    return () => clearInterval(timer)
  })

  $effect(() => {
    refresh()
  })

  // The host finished preparing; continue the flow this session started.
  $effect(() => {
    if (published && awaitingPublish && !hasCopy && !cloning) {
      awaitingPublish = false
      startClone()
    }
  })

  // The host failed to prepare — say so once, then stop waiting.
  $effect(() => {
    if (ws.git_serve === 'error' && awaitingPublish) {
      awaitingPublish = false
      showToast(t('workspace.publish_failed', { error: ws.git_serve_error ?? '' }), 'error')
    }
  })

  function message(e: unknown): string {
    return e instanceof Error ? e.message : String(e)
  }

  // setUp is the whole flow behind one button: report what publishing
  // costs, confirm it, ask the host to publish, then clone when it's ready.
  async function setUp() {
    busy = true
    try {
      const report = await InspectWorkspaceGitServe(brandSlug, streamSlug, projectSlug)
      if (!report.git_available) {
        showToast(t('workspace.no_git_on_server', { server: serverName }), 'error')
        return
      }
      if (!await confirmPublish(report.is_repo, report.files, report.bytes, report.uncommitted_paths)) return
      awaitingPublish = true
      await EnableWorkspaceGitServe(brandSlug, streamSlug, projectSlug)
    } catch (e) {
      awaitingPublish = false
      showToast(t('workspace.publish_failed', { error: message(e) }), 'error')
    } finally {
      busy = false
    }
  }

  // The size of the first commit is the one consequential fact here — a
  // folder with no .gitignore can be far larger than the user expects, and
  // finding that out afterwards is not good enough.
  async function confirmPublish(isRepo: boolean, files: number, bytes: number, uncommitted: number): Promise<boolean> {
    if (isRepo) {
      if (uncommitted > 0) {
        return showConfirm(t('workspace.publish_confirm_uncommitted', { server: serverName, count: String(uncommitted) }))
      }
      return showConfirm(t('workspace.publish_confirm_repo', { server: serverName }))
    }
    return showConfirm(t('workspace.publish_confirm_new', {
      server: serverName,
      files: files.toLocaleString(),
      size: formatBytes(bytes),
    }))
  }

  // parent = the folder to create the copy inside; '' uses the remembered
  // one (or a default under the user's home).
  async function startClone(parent = '') {
    busy = true
    try {
      await MaterializeWorkspace(ws.id, brandSlug, streamSlug, projectSlug, parent)
      await refresh()
    } catch (e) {
      showToast(t('workspace.clone_failed', { error: message(e) }), 'error')
    } finally {
      busy = false
    }
  }

  // The picker returns a folder that already EXISTS, so it names the parent
  // and the copy is created inside it — same shape as the default.
  async function cloneElsewhere() {
    try {
      const parent = await PickFolder(t('workspace.clone_pick_parent'))
      if (!parent) return
      await startClone(parent)
    } catch (e) {
      showToast(t('workspace.clone_failed', { error: message(e) }), 'error')
    }
  }

  // Every sync outcome is a status, so each gets its own sentence with the
  // server's name in it. Reporting git's raw refusal here would put "hint:
  // See the 'Note about fast-forwards'" in front of someone whose actual
  // situation is "your laptop and RIPPED have both changed".
  function reportSync(result: WorkspaceSyncResult, okKey: string) {
    switch (result.status) {
      case 'diverged':
        showToast(t('workspace.sync_diverged', { server: serverName }), 'error')
        break
      case 'server_dirty':
        showToast(t('workspace.sync_server_dirty', { server: serverName }), 'error')
        break
      case 'conflict':
        showToast(t('workspace.sync_conflict', { files: result.detail ?? '' }), 'error')
        break
      case 'up_to_date':
        showToast(t('workspace.sync_up_to_date'), 'success')
        break
      default:
        showToast(t(okKey, { server: serverName }), 'success')
    }
  }

  async function runSync(op: () => Promise<WorkspaceSyncResult>, okKey: string, failKey: string) {
    busy = true
    try {
      reportSync(await op(), okKey)
      await refresh()
    } catch (e) {
      showToast(t(failKey, { error: message(e) }), 'error')
    } finally {
      busy = false
    }
  }

  const pull = () => runSync(() => PullWorkspaceCheckout(ws.id), 'workspace.pull_done', 'workspace.pull_failed')
  const push = () => runSync(() => PushWorkspaceCheckout(ws.id), 'workspace.push_done', 'workspace.push_failed')
  const merge = () => runSync(() => MergeWorkspaceCheckout(ws.id), 'workspace.merge_done', 'workspace.merge_failed')

  async function forget() {
    if (!await showConfirm(t('workspace.forget_copy_confirm', { path: checkout?.local_path ?? '' }))) return
    try {
      await ForgetWorkspaceCheckout(ws.id)
      await refresh()
    } catch (e) {
      showToast(t('workspace.forget_copy_failed', { error: message(e) }), 'error')
    }
  }

  async function openCopy() {
    if (!checkout?.local_path) return
    try {
      await OpenWorkspacePath(checkout.local_path, '')
    } catch (e) {
      showToast(t('workspace.open_failed', { error: message(e) }), 'error')
    }
  }

  // Un-publishing is only offered once a copy exists to un-publish from —
  // otherwise the button is an invitation to break the thing you just set
  // up. It never touches the repository on either machine.
  async function unpublish() {
    if (!await showConfirm(t('workspace.unpublish_confirm', { server: serverName }))) return
    try {
      await DisableWorkspaceGitServe(brandSlug, streamSlug, projectSlug)
    } catch (e) {
      showToast(t('workspace.unpublish_failed', { error: message(e) }), 'error')
    }
  }
</script>

<section class="local-copy">
  <span class="label">{t('workspace.local_copy')}</span>

  {#if checkout && !checkout.git_available}
    <p class="warning"><AlertTriangle size={12} /> {t('workspace.no_git_on_device')}</p>

  {:else if hasCopy}
    <p class="path" title={checkout?.local_path}>{checkout?.local_path}</p>
    <p class="state">
      {#if checkout?.branch}<span class="branch">{checkout.branch}</span>{/if}
      {#if checkout?.dirty}<span class="chip dirty">{t('workspace.copy_dirty')}</span>{/if}
      {#if checkout && checkout.ahead > 0}<span class="chip">{t('workspace.copy_ahead', { count: String(checkout.ahead) })}</span>{/if}
      {#if checkout && checkout.behind > 0}<span class="chip">{t('workspace.copy_behind', { count: String(checkout.behind) })}</span>{/if}
      {#if checkout && !checkout.dirty && checkout.ahead === 0 && checkout.behind === 0}
        <span class="chip clean">{t('workspace.copy_in_sync')}</span>
      {/if}
    </p>
    {#if checkout?.diverged}
      <!-- Both sides moved. Neither Get nor Send can proceed, so say that
           once and offer the thing that resolves it, rather than leaving two
           buttons that each refuse for their own separate reason. -->
      <p class="warning"><AlertTriangle size={12} /> {t('workspace.diverged_hint', { server: serverName })}</p>
      <div class="action-row">
        <button class="btn primary" disabled={busy} onclick={merge}><GitMerge size={13} /> {t('workspace.merge')}</button>
      </div>
    {/if}
    <div class="action-row">
      <button class="btn" onclick={openCopy}><FolderOpen size={13} /> {t('workspace.open_copy')}</button>
      <button class="btn" disabled={busy} onclick={pull}><ArrowDownToLine size={13} /> {t('workspace.pull')}</button>
      <button class="btn" disabled={busy} onclick={push} title={t('workspace.push_hint', { server: serverName })}>
        <ArrowUpFromLine size={13} /> {t('workspace.push')}
      </button>
      <button class="btn subtle" onclick={forget} title={t('workspace.forget_copy_hint')}><Unlink size={13} /> {t('workspace.forget_copy')}</button>
    </div>
    <button class="link-btn" onclick={unpublish}>{t('workspace.unpublish')}</button>

  {:else if cloning}
    <p class="busy"><Loader size={13} class="spin" /> {t('workspace.cloning')}</p>
    {#if checkout?.progress}<p class="progress">{checkout.progress}</p>{/if}

  {:else if checkout?.status === 'error'}
    <p class="warning"><AlertTriangle size={12} /> {checkout.error}</p>
    <div class="action-row">
      <button class="btn" disabled={busy} onclick={() => startClone()}>{t('workspace.retry_clone')}</button>
    </div>

  {:else if publishing}
    <p class="busy"><Loader size={13} class="spin" /> {t('workspace.publishing', { server: serverName })}</p>
    <!-- A host that restarted mid-initialization leaves this state behind
         with nothing running. The retry is how the user gets out of it;
         preparing is idempotent, so pressing it during a live run is
         harmless. -->
    <button class="link-btn" disabled={busy} onclick={setUp}>{t('workspace.publish_retry')}</button>

  {:else if published}
    <p class="hint">{t('workspace.published_hint', { server: serverName })}</p>
    <div class="action-row">
      <button class="btn primary" disabled={busy} onclick={() => startClone()}>
        <HardDriveDownload size={13} /> {t('workspace.clone_here')}
      </button>
      <button class="btn" disabled={busy} onclick={cloneElsewhere}>{t('workspace.clone_choose_folder')}</button>
    </div>

  {:else}
    <p class="hint">{t('workspace.local_copy_hint', { server: serverName })}</p>
    {#if ws.git_serve === 'error' && ws.git_serve_error}
      <p class="warning"><AlertTriangle size={12} /> {ws.git_serve_error}</p>
    {/if}
    <div class="action-row">
      <button class="btn primary" disabled={busy} onclick={setUp}>
        <HardDriveDownload size={13} /> {t('workspace.setup_local_copy')}
      </button>
    </div>
  {/if}
</section>

<style>
  .local-copy {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.6rem 0 0;
    border-top: 1px solid var(--border);
  }
  .label {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .hint,
  .busy,
  .progress,
  .path,
  .state {
    margin: 0;
    font-size: 0.78rem;
    color: var(--text-muted);
  }
  .path {
    font-family: "Fira Code", "Consolas", monospace;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-primary);
  }
  .progress {
    font-family: "Fira Code", "Consolas", monospace;
    font-size: 0.72rem;
  }
  .busy {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .state {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.3rem;
  }
  .branch {
    font-family: "Fira Code", "Consolas", monospace;
    color: var(--text-primary);
  }
  .chip {
    padding: 0.05rem 0.35rem;
    border-radius: 999px;
    background: var(--bg-subtle);
    font-size: 0.7rem;
  }
  .chip.dirty {
    background: var(--warning-bg);
    color: var(--warning-text);
  }
  .chip.clean {
    color: var(--text-muted);
  }
  .warning {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    margin: 0;
    font-size: 0.75rem;
    color: var(--danger);
  }
  .action-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }
  .link-btn {
    align-self: flex-start;
    padding: 0;
    border: none;
    background: none;
    color: var(--text-muted);
    font-size: 0.72rem;
    text-decoration: underline;
    cursor: pointer;
  }
  .link-btn:hover {
    color: var(--text-primary);
  }
</style>
