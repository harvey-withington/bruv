import { describe, it, expect } from 'vitest'
import { parseModelRef, effectiveRef, refLabel, modelChoiceGroups, removeModel, removeRouter, isDanglingRef, describeDecision } from '@shared/modelRefs'
import type { LLMAccount, LLMRouting } from '@shared/types'

const t = (key: string, params?: Record<string, string | number>) =>
  params ? `${key}(${Object.entries(params).map(([k, v]) => `${k}=${v}`).join(',')})` : key

const accounts: LLMAccount[] = [
  { id: 'a2', label: 'Local', provider: 'ollama', api_key: '', base_url: '' },
  { id: 'a1', label: '', provider: 'anthropic', api_key: '', base_url: '' },
]

function routing(): LLMRouting {
  return {
    models: [
      { id: 'm1', account_id: 'a1', name: 'claude-sonnet-5', label: 'Sonnet', tier: 'balanced', supports_tools: true, enabled: true },
      { id: 'm2', account_id: 'a2', name: 'llama3.1', tier: 'fast', supports_tools: true, enabled: true },
      { id: 'm3', account_id: 'a1', name: 'claude-opus-5', tier: 'powerful', supports_tools: true, enabled: false },
    ],
    routers: [{ id: 'r1', name: 'Auto', kind: 'rules', thresholds: {}, fallback: 'model:m1', rules: [{ id: 'x', target: 'model:m1' }] }],
    default: 'model:m1',
    tasks: { agent_run: 'router:r1', card_chat: 'model:m1' },
  }
}

describe('modelRefs', () => {
  it('parses refs', () => {
    expect(parseModelRef('model:m1')).toEqual({ kind: 'model', id: 'm1' })
    expect(parseModelRef('router:')).toBeNull()
    expect(parseModelRef('weird:x')).toBeNull()
    expect(parseModelRef('')).toBeNull()
  })

  it('previews precedence: choice → task → default', () => {
    const r = routing()
    expect(effectiveRef('model:m2', 'agent_run', r)).toBe('model:m2')
    expect(effectiveRef('', 'agent_run', r)).toBe('router:r1')
    expect(effectiveRef(undefined, 'project_chat', r)).toBe('model:m1')
  })

  it('labels refs', () => {
    const r = routing()
    expect(refLabel('model:m1', r, t)).toBe('Sonnet')
    expect(refLabel('model:m2', r, t)).toBe('llama3.1')
    expect(refLabel('router:r1', r, t)).toBe('llm_routing.auto_router(name=Auto)')
    expect(refLabel('model:gone', r, t)).toBe('llm_routing.missing')
    expect(refLabel('', r, t, 'inherit')).toBe('inherit')
  })

  it('groups models by provider order and hides disabled unless selected', () => {
    const groups = modelChoiceGroups(routing(), accounts)
    expect(groups.map(g => g.id)).toEqual(['a2', 'a1'])
    expect(groups[1].label).toBe('anthropic')
    expect(groups[1].models.map(m => m.id)).toEqual(['m1'])
    expect(modelChoiceGroups(routing(), accounts, 'model:m3')[1].models.map(m => m.id)).toEqual(['m1', 'm3'])
  })

  it('removing a model clears every choice pointing at it', () => {
    const r = removeModel(routing(), 'm1')
    expect(r.models.map(m => m.id)).toEqual(['m2', 'm3'])
    expect(r.default).toBe('')
    expect(r.tasks).toEqual({ agent_run: 'router:r1' })
    expect(r.routers[0].fallback).toBe('tier:balanced')
    expect(r.routers[0].rules?.[0].target).toBe('tier:balanced')
  })

  it('removing a router clears task choices', () => {
    const r = removeRouter(routing(), 'r1')
    expect(r.routers).toEqual([])
    expect(r.tasks).toEqual({ card_chat: 'model:m1' })
    expect(isDanglingRef('router:r1', r)).toBe(true)
    expect(isDanglingRef('model:m1', r)).toBe(false)
  })

  it('explains a routed decision', () => {
    const text = describeDecision({
      model: 'llama3.1', provider: 'ollama', provider_label: 'Local', source: 'task',
      router_name: 'Auto', via: 'rule', rule_index: 2, band: 'low', score: 10,
      skipped: [{ source: 'override', ref: 'model:x', why: 'missing' }],
    }, t)
    expect(text.split('\n')).toEqual([
      'llm_routing.answered_by(model=llama3.1,provider=Local)',
      'llm_routing.source_task',
      'llm_routing.router_line(router=Auto,via=llm_routing.via_rule(rule=#2))',
      'llm_routing.complexity_line(band=llm_routing.band_low,score=10)',
      'llm_routing.skipped_line(source=llm_routing.source_override,why=llm_routing.skip_missing)',
    ])
  })
})
