// Mobile-side mirror of the desktop chat message shape. Matches the
// JSON the backend's chat history file on disk encodes — see
// internal/model/model.go's ChatMessage / ToolAction / PendingEdit
// structs.

import type { ChatNotice, RouteDecision } from '@shared/types'

export type ChatRole = 'user' | 'assistant' | 'system'

export type PendingEdit = {
  id: string
  tool: string
  input: Record<string, unknown>
  label: string
  detail: string
  /** `failed` = accepted, but the tool refused it at apply time; terminal. */
  status: 'pending' | 'accepted' | 'rejected' | 'failed'
  /** Why a `failed` edit did not apply. */
  error?: string
}

export type ToolAction = {
  tool: string
  input: unknown
  result: string
}

export type PinSuggestion = {
  category_id: string
  breadcrumb: string
  reason?: string
  status: string
  confidence?: string
}

export type ChatMessage = {
  id: string
  role: ChatRole | string
  content: string
  timestamp: string
  tool_actions?: ToolAction[]
  pin_suggestion?: PinSuggestion
  pending_edits?: PendingEdit[]
  /** Which model answered and why. */
  route?: RouteDecision
  /** Why the turn ended unusually (cut off, refused, stopped, over budget). */
  notice?: ChatNotice
}

export type ChatFile = {
  card_id?: string
  messages?: ChatMessage[]
}

export type LLMConfig = {
  provider?: string
  model?: string
  ai_mode?: string
  min_confidence?: string
  context?: string
  api_key?: string
  base_url?: string
}

/** Three-way mode the chat surface offers: edit (apply directly),
 *  suggest (propose, user reviews), chat (conversation only, no card
 *  mutations). Persisted globally via SetLLMConfig. */
export type AIMode = 'edit' | 'suggest' | 'chat'

/** Project-chat context level. Three values, persisted in localStorage
 *  globally (matches desktop). */
export type ProjectContextLevel = 'all' | 'metadata' | 'none'
