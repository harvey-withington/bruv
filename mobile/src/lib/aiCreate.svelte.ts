import { repoRPC } from './auth'
import { showToast } from './toast.svelte'
import { t } from './i18n.svelte'

// Quick capture's "Create with AI", the invisible half. The sheet has
// already created the Inbox card from the note; this hands the note to
// the card's chat in forced edit mode (PopulateCardWithAI) so the model
// titles, types, fills and pins the card directly — no chat sheet, no
// approval round-trip. The user is dropped onto the card while the
// turn runs; CardPage shows a banner while `isPopulating(id)` and the
// backend's card:updated events paint the edits in as they land. The
// exchange stays in the card's chat history for anyone curious.
//
// Module-level state, not component state: the capture sheet that
// starts the turn closes immediately, and the turn must outlive it.

let populating = $state<{ cardID: string; done: number } | null>(null)
// Bumped when a turn finishes so CardPage can do one final refetch
// (the last tool's card:updated can race the page's own load).
let finished = $state(0)

export function isPopulating(cardID: string): boolean {
  return populating?.cardID === cardID
}

export function populateFinished(): number {
  return finished
}

export function startPopulate(cardID: string, prompt: string): void {
  populating = { cardID, done: 0 }
  void repoRPC('PopulateCardWithAI', [cardID, prompt])
    .catch((err: unknown) => {
      showToast(`${t('capture.err_ai')} ${err instanceof Error ? err.message : ''}`.trim(), 'error')
    })
    .finally(() => {
      if (populating?.cardID === cardID) populating = null
      finished++
    })
}
