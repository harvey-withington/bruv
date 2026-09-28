<script lang="ts">
  import { FolderOpen, Play, GitCommitHorizontal } from 'lucide-svelte'
  import { OpenWorkspacePath, RunWorkspaceLaunchCommand, SetWorkspaceCommitOnSave, SetWorkspaceLaunchCommand } from '@shared/api'
  import type { Workspace, WorkspaceCheckoutInfo } from '@shared/types'
  import { t } from '../../lib/i18n.svelte'
  import { showToast } from '../../lib/toast.svelte'
  import EditableText from '../EditableText.svelte'
  import WorkspaceLocalCopy from './WorkspaceLocalCopy.svelte'

  // "On this device": everything about handing the workspace's files to
  // OTHER programs on this machine — open the folder, the launch command,
  // this device's clone on a remote connection, and commit-on-save so the
  // host tree stays clean for that clone to push into. Editing lives on
  // the card and in the Files section above; nothing here edits a file
  // (plan/2026-09-17 workspace files block.md §5).
  let { ws, brandSlug, streamSlug, projectSlug, serverIsThisMachine, serverName, deviceRoot, onWorkspaceChange, onCheckoutChange }: {
    ws: Workspace
    brandSlug: string
    streamSlug: string
    projectSlug: string
    serverIsThisMachine: boolean
    serverName: string
    /** The folder THIS device can open; undefined hides the actions. */
    deviceRoot: string | undefined
    onWorkspaceChange: (ws: Workspace) => void
    onCheckoutChange: (info: WorkspaceCheckoutInfo | null) => void
  } = $props()

  async function openFolder() {
    if (!deviceRoot) return
    try {
      await OpenWorkspacePath(deviceRoot, '')
    } catch (e) {
      showToast(t('workspace.open_failed', { error: message(e) }), 'error')
    }
  }

  async function runLaunch() {
    if (!deviceRoot || !ws.launch_command) return
    try {
      await RunWorkspaceLaunchCommand(deviceRoot, ws.launch_command)
    } catch (e) {
      showToast(t('workspace.launch_failed', { error: message(e) }), 'error')
    }
  }

  async function saveLaunchCommand(cmd: string) {
    try {
      onWorkspaceChange(await SetWorkspaceLaunchCommand(brandSlug, streamSlug, projectSlug, cmd))
    } catch {
      showToast(t('error.save_failed'), 'error')
    }
  }

  async function toggleCommitOnSave() {
    try {
      onWorkspaceChange(await SetWorkspaceCommitOnSave(brandSlug, streamSlug, projectSlug, !ws.commit_on_save))
    } catch {
      showToast(t('error.save_failed'), 'error')
    }
  }

  const message = (e: unknown) => e instanceof Error ? e.message : String(e)

  // Pick-to-fill launch suggestions, ordered by adapter (an Obsidian vault
  // most likely opens in Obsidian; a repo in an editor). Free text stays
  // the model — these just save you knowing that VS Code's binary is
  // `code`, not `vscode`.
  const launchSuggestions = $derived.by(() => {
    const obsidian = {
      label: t('workspace.launch_suggest_obsidian'),
      command: `obsidian://open?path=${encodeURIComponent(ws.origin.url ?? '')}`,
    }
    const vscode = { label: t('workspace.launch_suggest_vscode'), command: 'code .' }
    const terminal = { label: t('workspace.launch_suggest_terminal'), command: 'wt -d .' }
    return ws.adapter === 'obsidian-vault' ? [obsidian, vscode, terminal] : [vscode, terminal, obsidian]
  })
</script>

<div class="device">
  {#if deviceRoot}
    <div class="action-row">
      <button class="btn" onclick={openFolder}><FolderOpen size={13} /> {t('workspace.open_folder')}</button>
      {#if ws.launch_command}
        <button class="btn" onclick={runLaunch} title={ws.launch_command}><Play size={13} /> {t('workspace.launch')}</button>
      {/if}
    </div>
  {/if}
  {#if !serverIsThisMachine}
    <WorkspaceLocalCopy {ws} {brandSlug} {streamSlug} {projectSlug} {serverName} {onCheckoutChange} />
  {/if}
  <div class="launch">
    <span class="label">{t('workspace.launch_command')}</span>
    <EditableText
      value={ws.launch_command ?? ''}
      placeholder={t('workspace.launch_placeholder')}
      onSave={saveLaunchCommand}
    />
    {#if !ws.launch_command}
      <p class="hint">{t('workspace.launch_hint')}</p>
      <div class="chip-row">
        {#each launchSuggestions as s (s.label)}
          <button class="chip" title={s.command} onclick={() => saveLaunchCommand(s.command)}>{s.label}</button>
        {/each}
      </div>
    {/if}
  </div>
  {#if ws.git_serve === 'ready'}
    <!-- Only meaningful once clones exist: a push into a dirty host tree is
         refused, so BRUV's own writes on the host must not leave it dirty. -->
    <label class="toggle">
      <input type="checkbox" checked={ws.commit_on_save === true} onchange={toggleCommitOnSave} />
      <GitCommitHorizontal size={13} />
      <span>{t('workspace.commit_on_save')}</span>
    </label>
    <p class="hint">{t('workspace.commit_on_save_hint', { server: serverName })}</p>
  {/if}
</div>

<style>
  .device { display: flex; flex-direction: column; gap: 0.45rem; }
  .action-row { display: flex; gap: 0.4rem; flex-wrap: wrap; }
  .action-row .btn { padding: 0.28rem 0.6rem; font-size: 0.74rem; }
  .label {
    font-size: 0.66rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }
  .launch { display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.78rem; }
  .hint {
    margin: 0.1rem 0 0;
    font-size: 0.72rem;
    color: var(--text-muted);
    line-height: 1.4;
  }
  .chip-row { display: flex; flex-wrap: wrap; gap: 0.3rem; margin-top: 0.25rem; }
  .chip {
    padding: 0.2rem 0.55rem;
    font-size: 0.72rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    cursor: pointer;
    transition: color 0.12s, border-color 0.12s;
  }
  .chip:hover,
  .chip:focus-visible {
    color: var(--text-primary);
    border-color: var(--accent);
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.78rem;
    color: var(--text-body);
    cursor: pointer;
  }
  .toggle input { accent-color: var(--accent); margin: 0; }
</style>
