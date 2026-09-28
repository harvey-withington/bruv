// Groups an agent's grantable tools for the Agent tab's permission list.
//
// Contract (Harvey, 2026-09-29): the list must show a tick against EVERY
// permission the agent has, whatever is stored. So the rows come from
// the backend's own list of grantable tools (DescribeAgent options), not
// a hand-kept subset, and any granted id the backend doesn't offer (an
// offline MCP server's tool, a retired id) still appears — ticked, in a
// "granted but unavailable" group — so the user can see and revoke it.

import type { AgentToolOption, MCPServerView } from '@shared/types'

export type ToolRow = {
  id: string
  /** i18n key for the label, or null to show `name` as-is (MCP / unknown ids). */
  labelKey: string | null
  name: string
  descKey?: string
  granted: boolean
  available: boolean
}

export type ToolGroup = {
  key: string
  /** i18n key, or null for an MCP server (titled by its name). */
  titleKey: string | null
  title: string
  rows: ToolRow[]
}

// Built-ins keep their existing labels and descriptions.
const BUILTIN: Record<string, { labelKey: string; descKey: string }> = {
  web_fetch: { labelKey: 'agent.tool_web_fetch', descKey: 'agent.tool_web_fetch_desc' },
  web_search: { labelKey: 'agent.tool_web_search', descKey: 'agent.tool_web_search_desc' },
  http_request: { labelKey: 'agent.tool_http_request', descKey: 'agent.tool_http_request_desc' },
  update_self: { labelKey: 'agent.tool_update_self', descKey: 'agent.tool_update_self_desc' },
  notify: { labelKey: 'agent.tool_notify', descKey: 'agent.tool_notify_desc' },
}

// Native board tools, grouped by what they let an agent do.
const GROUPS: { key: string; titleKey: string; ids: string[] }[] = [
  { key: 'web', titleKey: 'agent.tool_group_web', ids: ['web_fetch', 'web_search', 'http_request'] },
  { key: 'self', titleKey: 'agent.tool_group_self', ids: ['update_self', 'notify'] },
  {
    key: 'read', titleKey: 'agent.tool_group_read',
    ids: ['get_card', 'search_cards', 'list_cards', 'recent_cards', 'list_categories', 'list_card_types', 'list_card_comments', 'get_card_attachment', 'get_card_agent'],
  },
  {
    key: 'write', titleKey: 'agent.tool_group_write',
    ids: ['create_card', 'update_card', 'set_card_title', 'set_card_description', 'set_card_type', 'set_card_due_date', 'set_card_fields', 'add_card_blocks', 'add_card_tags', 'add_card_comment', 'add_card_attachment'],
  },
  { key: 'filing', titleKey: 'agent.tool_group_filing', ids: ['pin_card', 'unpin_card', 'create_category'] },
  { key: 'board', titleKey: 'agent.tool_group_board', ids: ['list_brands', 'list_streams', 'list_projects', 'create_brand', 'create_stream', 'create_project'] },
]

/** i18n key for a native tool's label: agent.tool.<id>. */
export function nativeLabelKey(id: string): string {
  return `agent.tool.${id}`
}

export function buildToolGroups(options: AgentToolOption[], mcpServers: MCPServerView[], allowed: string[]): ToolGroup[] {
  const granted = new Set(allowed)
  const offered = new Map(options.map((o) => [o.id, o]))
  const placed = new Set<string>()
  const row = (id: string): ToolRow => {
    placed.add(id)
    const builtin = BUILTIN[id]
    return {
      id,
      labelKey: builtin?.labelKey ?? nativeLabelKey(id),
      name: id,
      descKey: builtin?.descKey,
      granted: granted.has(id),
      available: offered.get(id)?.ready ?? false,
    }
  }

  const mcpIds = new Set(mcpServers.flatMap((s) => s.tools.map((t) => t.namespace_id)))
  const groups: ToolGroup[] = []
  for (const g of GROUPS) {
    const rows = g.ids.filter((id) => offered.has(id)).map(row)
    if (rows.length) groups.push({ key: g.key, titleKey: g.titleKey, title: '', rows })
  }

  // Grantable tools this list doesn't know yet (a newer native tool).
  const other = options.filter((o) => !placed.has(o.id) && !mcpIds.has(o.id)).map((o) => row(o.id))
  if (other.length) groups.push({ key: 'other', titleKey: 'agent.tool_group_other', title: '', rows: other })

  // One group per MCP server that is set up, ready or not, listing its tools.
  for (const s of mcpServers) {
    if (!s.tools.length) continue
    const ready = s.health.status === 'ready'
    groups.push({
      key: `mcp:${s.spec.name}`,
      titleKey: null,
      title: s.spec.name,
      rows: s.tools.map((t) => {
        placed.add(t.namespace_id)
        return { id: t.namespace_id, labelKey: null, name: t.name, granted: granted.has(t.namespace_id), available: ready }
      }),
    })
  }

  // Granted ids nothing above shows — never hide a permission.
  const orphans = allowed.filter((id) => !placed.has(id))
  if (orphans.length) {
    groups.push({
      key: 'unavailable', titleKey: 'agent.tool_group_unavailable', title: '',
      rows: orphans.map((id) => ({ id, labelKey: null, name: id, granted: true, available: false })),
    })
  }
  return groups
}

/** Tick state of a group header: every row, some rows, or none granted. */
export function groupState(group: ToolGroup): 'all' | 'some' | 'none' {
  const n = group.rows.filter((r) => r.granted).length
  if (n === 0) return 'none'
  return n === group.rows.length ? 'all' : 'some'
}
