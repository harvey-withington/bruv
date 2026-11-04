// Which engine card chats use on desktop (shared/uncliChat), read the
// first time a card chat needs it and again whenever Settings saves the
// AI section, so switching takes effect without a reload.

import { GetLLMConfig, GetUncliStatus } from '@shared/api'
import { readCardChatEngine, type CardChatEngine } from '@shared/uncliChat'

export const chatEngine = $state<{ current: CardChatEngine }>({ current: { engine: 'builtin', fallback: null } })

let loading: Promise<void> | null = null
let loaded = false

/** Reads the choice the first time a card chat needs it. */
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
      chatEngine.current = await readCardChatEngine(GetLLMConfig, GetUncliStatus)
    } catch {
      chatEngine.current = { engine: 'builtin', fallback: null }
    } finally {
      loading = null
    }
  })()
  return loading
}
