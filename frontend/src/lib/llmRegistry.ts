// Desktop binding of the shared model registry (shared/llmRouting.svelte.ts).
import { GetLLMRegistry } from '@shared/api'
import { loadRegistryWith } from '@shared/llmRouting.svelte'

export { llmRegistry } from '@shared/llmRouting.svelte'

export function loadLLMRegistry(force = false): Promise<void> {
  return loadRegistryWith(GetLLMRegistry, force)
}
