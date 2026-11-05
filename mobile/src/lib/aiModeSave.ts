// Saves the chat mode (ai_mode) for both chat engines: the built-in
// chat's ChatModeButton and the UNCLI chat's message-box control.

import type { AIMode } from '@shared/aiMode'
import type { LLMConfig } from '@shared/types'
import { machineRPC } from './auth'

export async function saveAIMode(mode: AIMode): Promise<void> {
  const cfg = (await machineRPC<LLMConfig>('GetLLMConfig')) ?? ({} as LLMConfig)
  await machineRPC('SetLLMConfig', [{ ...cfg, ai_mode: mode }])
}
