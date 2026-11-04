// BRUV's chat mode (ai_mode in LLMConfig), one setting for both chat
// engines: Edit (the AI changes the board), Suggest (its changes are
// staged for the user's review) and Chat (it can't change the board).
// BRUV's own chat applies it in core/runtime/chat; the UNCLI engine's
// board-tool calls in internal/mcpserver/uncli.go. Both surfaces' mode
// buttons (ChatModeButton) cycle it.

export type AIMode = 'edit' | 'suggest' | 'chat'

/** The stored value as a mode; '' and unknown values are Edit. */
export function aiModeOf(value: string | undefined | null): AIMode {
  return value === 'chat' || value === 'suggest' ? value : 'edit'
}

/** The mode a click on the button moves to. */
export function nextAIMode(mode: AIMode): AIMode {
  return mode === 'edit' ? 'suggest' : mode === 'suggest' ? 'chat' : 'edit'
}

/** What the UNCLI chat's model is told about the mode with each turn,
 *  so it works with it rather than against a refusal. Model-facing, so
 *  not localized, like BRUV's own chat prompts. */
export function aiModeTurnContext(mode: AIMode): string {
  switch (mode) {
    case 'chat':
      return "The chat is in Chat mode: the bruv board tools can read but won't change anything. Describe changes instead of making them."
    case 'suggest':
      return 'The chat is in Suggest mode: changes you make with the bruv board tools are staged for the user to review and apply, not made at once. Say what you proposed.'
    default:
      return 'The chat is in Edit mode: changes you make with the bruv board tools apply at once.'
  }
}
