// Default model per LLM provider: the first model offered when a provider
// gets its first manually-added model. Mirrors DefaultModelForProvider in
// internal/config/llm_routing.go (migration + legacy config) — keep the
// two in sync when refreshing models.

export const DEFAULT_MODEL_FOR_PROVIDER: Readonly<Record<string, string>> = {
  openai: 'gpt-5.5',
  anthropic: 'claude-opus-5',
  ollama: 'llama3.1',
}

/** Default model for a provider, or an empty string for unknown providers. */
export function defaultModelForProvider(provider: string | undefined): string {
  return provider ? DEFAULT_MODEL_FOR_PROVIDER[provider] ?? '' : ''
}
