import { describe, it, expect } from 'vitest'
import { buildToolGroups, groupState } from './agentToolGroups'
import type { AgentToolOption, MCPServerView } from '@shared/types'

const opt = (id: string, ready = true): AgentToolOption => ({ id, ready })
const options = ['web_fetch', 'web_search', 'update_self', 'notify', 'get_card', 'create_card', 'update_card', 'pin_card'].map((id) => opt(id))
const server = (name: string, tools: string[], status = 'ready'): MCPServerView =>
  ({ spec: { name }, health: { status }, tools: tools.map((t) => ({ name: t, namespace_id: `${name}__${t}`, description: '' })) }) as unknown as MCPServerView

const granted = (groups: ReturnType<typeof buildToolGroups>) =>
  groups.flatMap((g) => g.rows).filter((r) => r.granted).map((r) => r.id).sort()

describe('buildToolGroups — every permission shows', () => {
  it('ticks exactly the granted tools, whatever group they fall in', () => {
    const groups = buildToolGroups(options, [], ['web_fetch', 'create_card', 'pin_card'])
    expect(granted(groups)).toEqual(['create_card', 'pin_card', 'web_fetch'])
  })

  it('shows nothing ticked for an agent with no tools', () => {
    expect(granted(buildToolGroups(options, [], []))).toEqual([])
  })

  it('lists each MCP tool, and a partly granted server is partly ticked', () => {
    const groups = buildToolGroups(options, [server('fs', ['read', 'write'])], ['fs__read'])
    const fs = groups.find((g) => g.key === 'mcp:fs')!
    expect(fs.rows.map((r) => [r.id, r.granted])).toEqual([['fs__read', true], ['fs__write', false]])
    expect(groupState(fs)).toBe('some')
  })

  it('never hides a grant the backend no longer offers', () => {
    const groups = buildToolGroups(options, [], ['web_fetch', 'gone_tool', 'offline__search'])
    const orphans = groups.find((g) => g.key === 'unavailable')!
    expect(orphans.rows.map((r) => r.id)).toEqual(['gone_tool', 'offline__search'])
    expect(orphans.rows.every((r) => r.granted && !r.available)).toBe(true)
  })

  it('puts an unrecognised native tool in "other" rather than dropping it', () => {
    const groups = buildToolGroups([...options, opt('brand_new_tool')], [], ['brand_new_tool'])
    expect(groups.find((g) => g.key === 'other')!.rows[0]).toMatchObject({ id: 'brand_new_tool', granted: true })
  })

  it('marks a tool unavailable when its MCP server is down, but still shows the grant', () => {
    const groups = buildToolGroups(options, [server('fs', ['read'], 'failed')], ['fs__read'])
    expect(groups.find((g) => g.key === 'mcp:fs')!.rows[0]).toMatchObject({ granted: true, available: false })
  })

  // An unapproved server is never started, so it lists no tools; a grant of
  // one of its tools must still show — ticked, unavailable — not vanish.
  it('shows grants of an unapproved MCP server as unavailable', () => {
    const groups = buildToolGroups(options, [server('shared', [], 'unapproved')], ['shared__search'])
    expect(groups.find((g) => g.key === 'unavailable')!.rows[0]).toMatchObject({ id: 'shared__search', granted: true, available: false })
  })
})
