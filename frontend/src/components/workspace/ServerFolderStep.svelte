<script lang="ts">
  // Attaching a workspace on a REMOTE connection.
  //
  // The folder is opened by the machine running the vault, which is not the
  // machine the user is sitting at — so the native picker (which browses
  // THIS disk) would hand the server a path it can't see. Until there's a
  // remote folder browser, the user types a path the server knows. The
  // desktop then clones a working copy for itself (WorkspaceLocalCopy).
  import { FolderInput } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import { focusOnMount } from '../../lib/actions'

  let { serverName, busy = false, onAttach }: {
    serverName: string
    busy?: boolean
    onAttach: (path: string) => void
  } = $props()

  let path = $state('')

  const canSubmit = $derived(path.trim().length > 0 && !busy)

  function submit() {
    if (!canSubmit) return
    onAttach(path.trim())
  }

  function onKeydown(e: KeyboardEvent) {
    // Enter commits, per the keyboard contract.
    if (e.key === 'Enter') {
      e.preventDefault()
      submit()
    }
  }
</script>

<div class="server-folder">
  <p class="hint">{t('workspace.server_path_hint', { server: serverName })}</p>
  <label>
    <span>{t('workspace.server_path_label', { server: serverName })}</span>
    <input
      type="text"
      bind:value={path}
      onkeydown={onKeydown}
      placeholder={t('workspace.server_path_placeholder')}
      spellcheck="false"
      use:focusOnMount
    />
  </label>
  <p class="note">{t('workspace.server_path_note')}</p>
  <div class="actions">
    <button class="btn primary" disabled={!canSubmit} onclick={submit}>
      <FolderInput size={14} /> {t('common.attach')}
    </button>
  </div>
</div>

<style>
  .server-folder {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    padding: 1rem;
  }
  .hint {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  input {
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-base);
    color: var(--text-primary);
    font-family: "Fira Code", "Consolas", monospace;
    font-size: 0.82rem;
  }
  input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .note {
    margin: 0;
    font-size: 0.75rem;
    color: var(--text-muted);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
  }
</style>
