// Half-typed chat messages, kept per conversation so switching card or
// project, or restarting the app, doesn't lose them. Saved on every
// change and cleared on send. Stored in localStorage: drafts belong to
// this device, not to the vault.

const PREFIX = 'bruv:draft:'

/** Scope key for a card's chat. */
export function cardDraftScope(cardId: string): string {
  return `card:${cardId}`
}

/** Scope key for a project's chat. */
export function projectDraftScope(projectPath: string): string {
  return `project:${projectPath}`
}

export function loadDraft(scope: string): string {
  try {
    return localStorage.getItem(PREFIX + scope) ?? ''
  } catch {
    return '' // storage unavailable (private mode): the box just starts empty
  }
}

export function saveDraft(scope: string, text: string): void {
  try {
    if (text.trim()) localStorage.setItem(PREFIX + scope, text)
    else localStorage.removeItem(PREFIX + scope)
  } catch {
    // Storage full or unavailable: the draft lives only in the box, as before.
  }
}
