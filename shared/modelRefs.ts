// Model choices (ModelRef strings) for both surfaces: parsing, the
// effective choice for a task, labels for pickers and chips, and the
// "why this model" explanation of a RouteDecision.
//
// The backend is the authority at request time (core/services/llm/
// select.go). These helpers only PREVIEW its precedence for display —
// they don't know about eligibility (disabled / tool support), so a
// chip can name a model the backend then skips; the message's route
// chip shows what actually answered.
//
// shared/ can't import a surface's i18n module, so callers pass `t`.
// Keys used live under llm_routing.* in both surfaces' en.json.

import type { TranslateFn } from './relativeTime'
import type { LLMModel, LLMProviderSummary, LLMRouter, LLMRouting, LLMTask, ModelRef, ModelTier, RouteDecision } from './types'

export type ModelRefKind = 'model' | 'router' | 'tier'

/** Fills in anything a backend response may omit (an older server, a
 * nil Go slice or map), so pickers and Settings can index freely. Apply
 * to every LLMRouting that comes off the wire. */
export function normalizeRouting(r: Partial<LLMRouting> | null | undefined): LLMRouting {
  return {
    models: r?.models ?? [],
    routers: (r?.routers ?? []).map(x => ({ ...x, thresholds: x.thresholds ?? {} })),
    default: r?.default ?? '',
    tasks: r?.tasks ?? {},
  }
}

/** Task ids in settings order. Mirrors llmsvc.Tasks. */
export const LLM_TASKS: readonly LLMTask[] = ['card_chat', 'project_chat', 'card_populate', 'agent_run']

export const MODEL_TIERS: readonly ModelTier[] = ['fast', 'balanced', 'powerful']

export function parseModelRef(ref: ModelRef | string | undefined): { kind: ModelRefKind; id: string } | null {
  if (!ref) return null
  const i = ref.indexOf(':')
  if (i < 0) return null
  const kind = ref.slice(0, i)
  const id = ref.slice(i + 1)
  if (!id || (kind !== 'model' && kind !== 'router' && kind !== 'tier')) return null
  return { kind, id }
}

export function modelRef(kind: ModelRefKind, id: string): ModelRef {
  return `${kind}:${id}` as ModelRef
}

/** The choice that will seed selection for a task: the invocation's own
 * choice, else the task assignment, else the global default. */
export function effectiveRef(choice: ModelRef | undefined, task: LLMTask, routing: LLMRouting): ModelRef {
  return choice || routing.tasks[task] || routing.default || ''
}

export function modelDisplayLabel(m: Pick<LLMModel, 'label' | 'name'>): string {
  return m.label || m.name
}

export function accountDisplayLabel(a: Pick<LLMProviderSummary, 'label' | 'provider'>): string {
  return a.label || a.provider
}

/** Short label for a choice: the model's name, "Auto · <router>", a tier,
 * or "Missing" for a dangling ref. '' labels as `emptyLabel`. */
export function refLabel(ref: ModelRef | undefined, routing: LLMRouting, t: TranslateFn, emptyLabel = ''): string {
  const parsed = parseModelRef(ref)
  if (!parsed) return emptyLabel
  switch (parsed.kind) {
    case 'model': {
      const m = routing.models.find(x => x.id === parsed.id)
      return m ? modelDisplayLabel(m) : t('llm_routing.missing')
    }
    case 'router': {
      const r = routing.routers.find(x => x.id === parsed.id)
      return r ? t('llm_routing.auto_router', { name: r.name }) : t('llm_routing.missing')
    }
    case 'tier':
      return t(`llm_routing.tier_${parsed.id}`)
  }
}

export type ModelChoiceGroup = { id: string; label: string; models: LLMModel[] }

/** Models grouped under their providers, in provider order. Disabled
 * models are left out unless they are the current value, so a picker
 * never silently changes what's stored. */
export function modelChoiceGroups(routing: LLMRouting, accounts: LLMProviderSummary[], current?: ModelRef): ModelChoiceGroup[] {
  const currentId = parseModelRef(current)?.kind === 'model' ? parseModelRef(current)!.id : ''
  return accounts
    .map(a => ({
      id: a.id,
      label: accountDisplayLabel(a),
      models: routing.models.filter(m => m.account_id === a.id && (m.enabled || m.id === currentId)),
    }))
    .filter(g => g.models.length > 0)
}

/** True when a ref names nothing in the registry (deleted model/router). */
export function isDanglingRef(ref: ModelRef | undefined, routing: LLMRouting): boolean {
  const parsed = parseModelRef(ref)
  if (!parsed) return false
  if (parsed.kind === 'model') return !routing.models.some(m => m.id === parsed.id)
  if (parsed.kind === 'router') return !routing.routers.some(r => r.id === parsed.id)
  return false
}

/** Removes a model from the registry and clears every choice pointing at
 * it, so no setting is left dangling. */
export function removeModel(routing: LLMRouting, modelId: string): LLMRouting {
  const gone = modelRef('model', modelId)
  return clearRef({ ...routing, models: routing.models.filter(m => m.id !== modelId) }, gone)
}

/** Removes a router and clears every choice pointing at it. */
export function removeRouter(routing: LLMRouting, routerId: string): LLMRouting {
  const gone = modelRef('router', routerId)
  return clearRef({ ...routing, routers: routing.routers.filter(r => r.id !== routerId) }, gone)
}

function clearRef(routing: LLMRouting, gone: ModelRef): LLMRouting {
  const tasks: Record<string, ModelRef> = {}
  for (const [task, ref] of Object.entries(routing.tasks)) {
    if (ref !== gone) tasks[task] = ref
  }
  return {
    ...routing,
    tasks,
    default: routing.default === gone ? '' : routing.default,
    routers: routing.routers.map((r): LLMRouter => ({
      ...r,
      fallback: r.fallback === gone ? modelRef('tier', 'balanced') : r.fallback,
      rules: r.rules?.map(rule => rule.target === gone ? { ...rule, target: modelRef('tier', 'balanced') } : rule),
    })),
  }
}

/** One-line label for the model that answered a message. */
export function decisionLabel(d: RouteDecision): string {
  return d.model_label || d.model
}

/** Multi-line explanation of a RouteDecision for a tooltip / detail row. */
export function describeDecision(d: RouteDecision, t: TranslateFn): string {
  const lines: string[] = []
  const provider = d.provider_label || d.provider
  lines.push(t('llm_routing.answered_by', { model: decisionLabel(d), provider }))
  lines.push(t(`llm_routing.source_${d.source}`))
  if (d.router_name) {
    const via = d.via === 'rule'
      ? t('llm_routing.via_rule', { rule: d.rule_name || `#${d.rule_index ?? '?'}` })
      : t(`llm_routing.via_${d.via ?? 'first'}`)
    lines.push(t('llm_routing.router_line', { router: d.router_name, via }))
  }
  if (d.score !== undefined && d.band) {
    lines.push(t('llm_routing.complexity_line', { band: t(`llm_routing.band_${d.band}`), score: d.score }))
  }
  for (const s of d.skipped ?? []) {
    lines.push(t('llm_routing.skipped_line', { source: t(`llm_routing.source_${s.source}`), why: t(`llm_routing.skip_${s.why}`) }))
  }
  return lines.join('\n')
}

/** An agent's pre-routing account/model pair as a registry choice: the
 * model on that account with that name, or '' when there is no match
 * (the backend then runs the name ad hoc on the account). */
export function refFromLegacyPair(accountId: string, name: string, routing: LLMRouting): ModelRef {
  if (!accountId && !name) return ''
  const m = routing.models.find(x => x.account_id === accountId && (!name || x.name === name))
  return m ? modelRef('model', m.id) : ''
}
