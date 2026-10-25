// The Settings dialog's tabs and its search index: each searchable field
// has a key (what the tab components gate on) and the words that find it.

export type SettingsTabId = 'general' | 'ai' | 'notifications' | 'templates' | 'capture'

export const SETTINGS_TABS: { id: SettingsTabId; labelKey: string }[] = [
  { id: 'general', labelKey: 'prefs.tab_general' },
  { id: 'ai', labelKey: 'prefs.tab_ai' },
  { id: 'notifications', labelKey: 'prefs.tab_notifications' },
  { id: 'templates', labelKey: 'prefs.tab_templates' },
  { id: 'capture', labelKey: 'prefs.tab_capture' },
]

type SettingsField = { tab: SettingsTabId; key: string; label: string }

const FIELDS: SettingsField[] = [
  { tab: 'general', key: 'reopen_last_repo', label: 'reopen last repository launch' },
  { tab: 'general', key: 'theme', label: 'theme dark light' },
  { tab: 'general', key: 'locale', label: 'language locale' },
  { tab: 'general', key: 'confirm_before_delete', label: 'confirm before deleting' },
  { tab: 'general', key: 'sidebar_width', label: 'sidebar width' },
  { tab: 'general', key: 'type_badge_display', label: 'category type badges' },
  { tab: 'general', key: 'default_category_name', label: 'default category name' },
  { tab: 'general', key: 'inbox_recent_cards_limit', label: 'inbox recently updated card limit' },
  { tab: 'general', key: 'inbox_activity_limit', label: 'inbox activity limit' },
  { tab: 'general', key: 'sidebar_collapse_default', label: 'sidebar collapsed collapse tree startup' },
  { tab: 'general', key: 'local_server_port', label: 'local server port fixed stable clipper pairing' },
  { tab: 'ai', key: 'accounts', label: 'ai providers accounts provider openai anthropic ollama openrouter lm studio compatible local api key models find discover tier tools' },
  { tab: 'ai', key: 'use_for', label: 'use for model per task default card chat project chat create with ai agents assign' },
  { tab: 'ai', key: 'routers', label: 'routers routing auto automatic rules complexity heuristic fallback keywords try' },
  { tab: 'ai', key: 'ai_mode', label: 'ai mode chat edit card fields' },
  { tab: 'ai', key: 'min_confidence', label: 'minimum confidence ai suggestion pin threshold' },
  { tab: 'ai', key: 'chat_budget', label: 'response budget tokens output limit cost warn stop chat reply' },
  { tab: 'ai', key: 'prompt_cache', label: 'conversation prompt cache caching duration ttl hour minutes cost price claude anthropic' },
  { tab: 'ai', key: 'context', label: 'ai context additional' },
  { tab: 'notifications', key: 'system_enabled', label: 'desktop system notifications test' },
  { tab: 'notifications', key: 'smtp', label: 'email smtp host server port username password recipient to from address tls' },
  { tab: 'notifications', key: 'webhook', label: 'webhook url post auth header' },
  { tab: 'notifications', key: 'due_date', label: 'due date notifications reminder overdue' },
  { tab: 'templates', key: 'slide_templates', label: 'slide templates auto matching priority url pattern regex social post clipper' },
  { tab: 'capture', key: 'capture_video', label: 'capture video quality mode best fit smallest link skip budget mb size' },
  { tab: 'capture', key: 'capture_images', label: 'capture images gallery all first only link skip' },
  { tab: 'capture', key: 'capture_ask', label: 'capture ask before dialog always never triggers when it matters' },
  { tab: 'capture', key: 'capture_triggers', label: 'capture triggers consequential video size gallery count unsupported platform blocked' },
]

/** Fields owned by CaptureSettingsSection, gated as one unit. */
export const CAPTURE_KEYS = ['capture_video', 'capture_images', 'capture_ask', 'capture_triggers']

/** Keys matching a search query, or null when not searching (all visible). */
export function matchSettings(query: string): Set<string> | null {
  const q = query.trim().toLowerCase()
  if (!q) return null
  return new Set(FIELDS.filter(f => f.label.includes(q) || f.key.includes(q)).map(f => f.key))
}

/** Tabs with at least one matching field, or null when not searching. */
export function tabsMatching(keys: Set<string> | null): Set<SettingsTabId> | null {
  if (!keys) return null
  return new Set(FIELDS.filter(f => keys.has(f.key)).map(f => f.tab))
}

/** A visibility test for tab components: everything shows when not searching. */
export type FieldVisible = (key: string) => boolean
