// Saves the chat mode (ai_mode) for both chat engines: the built-in
// chat's ChatModeButton and the UNCLI chat's message-box control.

import { GetLLMConfig, SetLLMConfig } from '@shared/api'
import type { AIMode } from '@shared/aiMode'

export async function saveAIMode(mode: AIMode): Promise<void> {
  const cfg = await GetLLMConfig()
  await SetLLMConfig({ ...cfg, ai_mode: mode })
}
