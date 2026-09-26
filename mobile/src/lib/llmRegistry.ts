// Mobile binding of the shared model registry (shared/llmRouting.svelte.ts).
import { machineRPC } from './auth'
import { loadRegistryWith } from '@shared/llmRouting.svelte'
import type { LLMRegistryView } from '@shared/types'

export { llmRegistry } from '@shared/llmRouting.svelte'

export function loadLLMRegistry(force = false): Promise<void> {
  return loadRegistryWith(() => machineRPC<LLMRegistryView>('GetLLMRegistry'), force)
}
