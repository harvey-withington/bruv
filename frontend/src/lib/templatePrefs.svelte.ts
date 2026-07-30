// Vault-level slide-template preferences (Auto matching: priority order +
// per-template urlHint overrides) as a shared reactive store, so the editor
// preview, deck block, and the Settings section all resolve Auto templates
// from the same state and react to saves immediately.

import type { TemplatePrefs } from '@shared/types'
import { GetTemplatePrefs, SetTemplatePrefs } from '@shared/api'

const state = $state<{ prefs: TemplatePrefs }>({ prefs: {} })

export function templatePrefs(): TemplatePrefs {
  return state.prefs
}

// Always fetches — prefs are per-vault and tiny, and consumers load on
// mount/open, so a stale cache across repo switches isn't worth the risk.
export async function loadTemplatePrefs(): Promise<void> {
  state.prefs = (await GetTemplatePrefs()) ?? {}
}

// Persist-then-update: the store only reflects what the server accepted.
// Callers surface errors (toast) — this just propagates them.
export async function saveTemplatePrefs(next: TemplatePrefs): Promise<void> {
  await SetTemplatePrefs(next)
  state.prefs = next
}
