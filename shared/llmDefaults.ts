// Default model per LLM provider, used for placeholders when an account has
// no explicit model. Mirrors DefaultModelForProvider in
// core/services/llm/service.go, which is the authority at request time —
// keep the two in sync when refreshing models.

export const DEFAULT_MODEL_FOR_PROVIDER: Readonly<Record<string, string>> = {
  openai: 'gpt-5.5',
  anthropic: 'claude-opus-5',
  ollama: 'llama3.1',
}

/** Default model for a provider, or an empty string for unknown providers. */
export function defaultModelForProvider(provider: string | undefined): string {
  return provider ? DEFAULT_MODEL_FOR_PROVIDER[provider] ?? '' : ''
}
