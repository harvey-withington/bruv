// The model registry as the model pickers see it: routing (models,
// routers, assignments) plus the providers for group labels and order —
// never their keys (GetLLMRegistry strips them). Shared by every picker
// on a surface; Settings reloads it after a save so open chats pick up
// new models.
//
// Each surface fetches through its own RPC layer, so the loader is
// passed in (desktop: frontend/src/lib/llmRegistry.ts, mobile:
// mobile/src/lib/llmRegistry.ts).

import type { LLMProviderSummary, LLMRegistryView, LLMRouting } from './types'
import { normalizeRouting } from './modelRefs'

export const llmRegistry = $state<{ routing: LLMRouting; accounts: LLMProviderSummary[]; loaded: boolean }>({
  routing: { models: [], routers: [], default: '', tasks: {} },
  accounts: [],
  loaded: false,
})

let inflight: Promise<void> | null = null

/** Loads the registry if it hasn't been; `force` reloads. Rejects on
 * RPC failure so callers can surface it. */
export function loadRegistryWith(load: () => Promise<LLMRegistryView>, force = false): Promise<void> {
  if (llmRegistry.loaded && !force) return Promise.resolve()
  if (inflight) return inflight
  inflight = load()
    .then(view => {
      llmRegistry.routing = normalizeRouting(view.routing)
      llmRegistry.accounts = view.providers ?? []
      llmRegistry.loaded = true
    })
    .finally(() => { inflight = null })
  return inflight
}
