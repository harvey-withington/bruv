<script lang="ts">
  // The Agent tab's permission list: one row per tool the agent can be
  // granted (from DescribeAgent), grouped, each ticked iff granted. Rows
  // come from lib/agentToolGroups.ts, which guarantees every granted id
  // is shown — see its header for the contract.
  import { t } from '../lib/i18n.svelte'
  import type { AgentToolOption, MCPServerView } from '@shared/types'
  import { buildToolGroups, groupState, type ToolGroup, type ToolRow } from '../lib/agentToolGroups'

  let { allowedTools = $bindable(), options, mcpServers, onchange }: {
    allowedTools: string[]
    options: AgentToolOption[]
    mcpServers: MCPServerView[]
    /** Called after the user grants or revokes a tool. */
    onchange: () => void
  } = $props()

  const groups = $derived(buildToolGroups(options, mcpServers, allowedTools))

  function setGranted(ids: string[], grant: boolean) {
    allowedTools = grant
      ? [...new Set([...allowedTools, ...ids])]
      : allowedTools.filter((id) => !ids.includes(id))
    onchange()
  }

  function toggleGroup(group: ToolGroup) {
    // Header ticks every row it can; a partly ticked group fills up.
    const grantable = group.rows.filter((r) => r.available || r.granted).map((r) => r.id)
    setGranted(grantable, groupState(group) !== 'all')
  }

  function groupTitle(group: ToolGroup): string {
    return group.titleKey ? t(group.titleKey) : group.title
  }

  function rowLabel(row: ToolRow): string {
    if (!row.labelKey) return row.name
    const label = t(row.labelKey)
    return label === row.labelKey ? row.name : label // unknown key → the id
  }
</script>

<div class="tools-list">
  {#if !allowedTools.length}
    <p class="no-tools">{t('agent.no_tools_granted')}</p>
  {/if}
  {#each groups as group (group.key)}
    {@const state = groupState(group)}
    <div class="tool-group" class:mcp-group={group.key.startsWith('mcp:')} class:orphan-group={group.key === 'unavailable'}>
      <label class="tool-group-header">
        <input
          type="checkbox"
          checked={state === 'all'}
          indeterminate={state === 'some'}
          onchange={() => toggleGroup(group)}
        />
        <span class="tool-group-title">{groupTitle(group)}</span>
      </label>
      {#each group.rows as row (row.id)}
        <label class="tool-item" class:unavailable={!row.available}>
          <input
            type="checkbox"
            checked={row.granted}
            disabled={!row.available && !row.granted}
            onchange={() => setGranted([row.id], !row.granted)}
          />
          <span class="tool-info">
            <span class="tool-name">{rowLabel(row)}</span>
            {#if row.descKey}
              <span class="tool-desc">{t(row.descKey)}</span>
            {/if}
            {#if !row.available}
              <span class="tool-desc warn">{t('agent.tool_unavailable')}</span>
            {/if}
          </span>
        </label>
      {/each}
    </div>
  {/each}
</div>

<style>
  .tools-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .no-tools {
    margin: 0 0 0.4rem;
    font-size: 0.72rem;
    color: var(--warning);
  }
  .tool-group {
    margin-bottom: 0.4rem;
  }
  .tool-group-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.4rem;
    cursor: pointer;
  }
  .tool-group-title {
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-faint);
  }
  .mcp-group,
  .orphan-group {
    border-top: 1px solid var(--border-muted);
    padding-top: 0.5rem;
    margin-top: 0.3rem;
  }
  .tool-item {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin-left: 0.6rem;
    padding: 0.3rem 0.5rem;
    border-radius: 4px;
    cursor: pointer;
    transition: background var(--duration-fast);
    flex-shrink: 0;
  }
  .tool-item:hover { background: var(--bg-hover); }
  .tool-item.unavailable { opacity: 0.75; }
  .tool-item input { margin-top: 0.15rem; }
  .tool-info { display: flex; flex-direction: column; }
  .tool-name { font-size: 0.8rem; font-weight: 500; color: var(--text-body); }
  .tool-desc { font-size: 0.7rem; color: var(--text-muted); }
  .tool-desc.warn { color: var(--warning); }
</style>
