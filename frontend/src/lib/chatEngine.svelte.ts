// Which engine BRUV's chats (cards and projects) use on desktop
// (shared/uncliChat), read the first time a chat needs it and again
// whenever Settings saves the AI section, so switching takes effect
// without a reload.

import { GetLLMConfig, GetUncliStatus } from '@shared/api'
import { readChatEngine, type ChatEngineChoice } from '@shared/uncliChat'

export const chatEngine = $state<{ current: ChatEngineChoice }>({ current: { engine: 'builtin', fallback: null } })

let loading: Promise<void> | null = null
let loaded = false

/** Reads the choice the first time a chat needs it. */
export function ensureChatEngine(): void {
  if (!loaded) void loadChatEngine()
}

/** (Re)reads the choice. If the setting itself can't be read, chats stay
 *  built-in, as before the opt-in existed; the chat's own load surfaces
 *  that failure. */
export function loadChatEngine(): Promise<void> {
  loading ??= (async () => {
    loaded = true
    try {
      chatEngine.current = await readChatEngine(GetLLMConfig, GetUncliStatus)
    } catch {
      chatEngine.current = { engine: 'builtin', fallback: null }
    } finally {
      loading = null
    }
  })()
  return loading
}

/** The built-in chat's banner when the user chose UNCLI but it can't
 *  run here ('' otherwise), via the surface's t. */
export function engineFallbackText(t: (key: string, params?: Record<string, string>) => string): string {
  const c = chatEngine.current
  return c.engine === 'builtin' && c.fallback ? t(c.fallback.key, c.fallback.params) : ''
}
