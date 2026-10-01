<script lang="ts">
  // Shown on an MCP server row whose status is 'unapproved': enabled in
  // the repo's mcp_servers.json (which travels with shared repos) but not
  // approved on this device, so it isn't run. Approve shows the exact
  // command first, then approves the fingerprint the user saw.
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { ApproveMCPServer } from '@shared/api'
  import type { MCPServerView } from '@shared/types'
  import { formatMCPCommand } from '../lib/mcpCommand'
  import { ShieldAlert } from 'lucide-svelte'

  let { view, onApproved }: { view: MCPServerView; onApproved: () => void | Promise<void> } = $props()

  let busy = $state(false)

  async function approve() {
    const envNames = view.spec.env_names ?? []
    const env = envNames.length > 0 ? t('mcp.approve_env', { names: envNames.join(', ') }) : ''
    const ok = await showConfirm(t('mcp.approve_confirm', { name: view.spec.name, command: formatMCPCommand(view.spec), env }))
    if (!ok) return
    busy = true
    try {
      await ApproveMCPServer(view.spec.name, view.fingerprint)
      showToast(t('mcp.approve_success'), 'success')
      await onApproved()
    } catch (e) {
      showToast(t('mcp.approve_failed') + ': ' + String(e), 'error')
    } finally {
      busy = false
    }
  }
</script>

<div class="unapproved-notice">
  <ShieldAlert size={13} />
  <span class="notice-text">{t('mcp.unapproved_notice')}</span>
  <button class="approve-btn" onclick={approve} disabled={busy}>{t('mcp.approve')}</button>
</div>

<style>
  .unapproved-notice {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    margin-top: 4px;
    padding: 6px 8px;
    font-size: 11px;
    line-height: 1.4;
    color: var(--warning-text);
    background: var(--warning-bg);
    border: 1px solid var(--warning-border);
    border-radius: var(--radius);
  }
  .notice-text { flex: 1; }
  .approve-btn {
    flex-shrink: 0;
    padding: 2px 10px;
    font-size: 11px;
    border-radius: var(--radius);
    border: 1px solid var(--warning);
    background: var(--warning);
    color: var(--on-color);
    cursor: pointer;
  }
  .approve-btn:hover { filter: brightness(1.1); }
  .approve-btn:disabled { opacity: 0.6; cursor: default; }
</style>
