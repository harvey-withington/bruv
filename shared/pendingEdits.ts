// Edits waiting for the user's review across a chat's history: what the
// UNCLI chat's review banner (UncliPendingEdits) lists. In Suggest mode
// the UNCLI engine stages its board writes on the chat's own history
// (supervisor.StageUncliEdit), so they're reviewed and applied with the
// same Apply(Project)PendingEdits as BRUV's own chat.

type EditLike = { id: string; status: string }
type MessageLike<E extends EditLike> = { id: string; pending_edits?: E[] }

/** The messages that still have edits waiting, with just those edits. */
export function pendingEditGroups<E extends EditLike, M extends MessageLike<E>>(messages: M[]): { messageId: string; edits: E[] }[] {
  const groups: { messageId: string; edits: E[] }[] = []
  for (const m of messages) {
    const edits = (m.pending_edits ?? []).filter(e => e.status === 'pending')
    if (edits.length) groups.push({ messageId: m.id, edits })
  }
  return groups
}

/** How many of acceptIDs failed to apply, after an apply. */
export function failedEditCount<E extends EditLike, M extends MessageLike<E>>(messages: M[], acceptIDs: string[]): number {
  const accepted = new Set(acceptIDs)
  return messages.reduce((n, m) => n + (m.pending_edits ?? []).filter(e => accepted.has(e.id) && e.status === 'failed').length, 0)
}
