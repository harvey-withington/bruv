// The Settings dialog's working copy: every value the tabs edit, the
// per-section load/save wiring (lib/settingsSections) and Save itself.
// The dialog is the shell; each tab component edits `form` directly.
//
// Server-zone prefs come over RPC; per-device prefs (theme, locale,
// layout) come from the local shell / localStorage. The initial values
// below are placeholders only — a section that fails to load is never
// saved, so they can't overwrite real settings.

import { GetPreferences, SetPreferences, GetUIPreferences, SetUIPreferences, GetLLMConfig, SetLLMConfig, GetNotifyConfig, SetNotifyConfig, GetLLMAccounts, SaveLLMAccounts, GetLLMRouting, SaveLLMRouting, GetDueDateSettings, SaveDueDateSettings, GetCapturePrefs, SetCapturePrefs } from '@shared/api'
import type { CapturePrefs, LLMAccount, LLMRouting, ChatBudgetMode } from '@shared/types'
import { CHAT_BUDGET_DEFAULT, clampChatBudget } from '@shared/chatTurn.svelte'
import { normalizeRouting } from '@shared/modelRefs'
import { t, setLocale } from './i18n.svelte'
import { theme } from './theme.svelte'
import { nav, prefs as prefsStore } from './store.svelte'
import { loadLLMRegistry } from './llmRegistry'
import { SettingsSections } from './settingsSections.svelte'

export type SettingsSectionId = 'prefs' | 'ui' | 'llmConfig' | 'llmProviders' | 'notify' | 'dueDate' | 'capture'

export type TypeBadgeDisplay = 'text' | 'color' | 'hidden'
export type DueDateThreshold = '24h' | '1h' | '0' | 'overdue'
export type DueDateChannel = 'system' | 'email' | 'webhook'

export class SettingsForm {
  prefs = $state({
    reopen_last_repo: false,
    theme: 'dark',
    locale: 'en',
    confirm_before_delete: true,
    sidebar_width: 260,
    type_badge_display: 'color' as TypeBadgeDisplay,
    default_category_name: t('prefs.default_category_placeholder'),
    inbox_recent_cards_limit: 21,
    inbox_activity_limit: 25,
    sidebar_collapse_default: false,
    local_server_port: 0,
  })

  llm = $state({
    context: '',
    provider: '',
    model: '',
    api_key: '',
    base_url: '',
    ai_mode: 'edit',
    min_confidence: '',
    chat_budget_mode: 'warn' as ChatBudgetMode,
    chat_budget_tokens: CHAT_BUDGET_DEFAULT,
  })

  llmAccounts = $state<LLMAccount[]>([])
  llmRouting = $state<LLMRouting>({ models: [], routers: [], default: '', tasks: {} })

  notify = $state({
    system_enabled: true, // retained for backward-compat with NotifyConfig shape; no longer consulted by the backend
    smtp_host: '',
    smtp_port: 587,
    smtp_username: '',
    smtp_password: '',
    smtp_from_addr: '',
    smtp_to_addr: '',
    smtp_tls: true,
    webhook_url: '',
    webhook_auth_header: '',
  })

  dueDate = $state({
    enabled: true,
    thresholds: { '24h': true, '1h': true, '0': true, overdue: false } as Record<DueDateThreshold, boolean>,
    channels: { system: true, email: false, webhook: false } as Record<DueDateChannel, boolean>,
  })

  // Mirrors internal/repo/capture_prefs.go DefaultCapturePrefs.
  capture = $state<CapturePrefs>({
    videoMode: 'fit',
    videoBudgetMB: 50,
    imageMode: 'all',
    askMode: 'triggers',
    triggers: { videoOverMB: 50, galleryOverCount: 8, unsupportedUrl: true, blocked: true, pinMayReject: true },
  })

  saving = $state(false)

  readonly labels: Record<SettingsSectionId, string> = {
    prefs: t('prefs.section_prefs'),
    ui: t('prefs.section_ui'),
    llmConfig: t('prefs.section_llm_config'),
    llmProviders: t('prefs.section_llm_providers'),
    notify: t('prefs.section_notify'),
    dueDate: t('prefs.section_due_date'),
    capture: t('prefs.section_capture'),
  }

  readonly sections = new SettingsSections<SettingsSectionId>({
    prefs: {
      load: async () => {
        const p = await GetPreferences()
        this.prefs.default_category_name = p?.default_category_name || t('prefs.default_category_placeholder')
      },
      save: () => SetPreferences({ default_category_name: this.prefs.default_category_name }),
    },
    ui: {
      load: async () => {
        const ui = await GetUIPreferences()
        if (!ui) return
        const p = this.prefs
        p.reopen_last_repo = ui.reopen_last_repo ?? false
        p.theme = ui.theme || 'dark'
        p.locale = ui.locale || 'en'
        p.confirm_before_delete = ui.confirm_before_delete ?? true
        p.sidebar_width = nav.sidebarWidth
        p.type_badge_display = (ui.type_badge_display || 'color') as TypeBadgeDisplay
        p.inbox_recent_cards_limit = ui.inbox_recent_cards_limit || 21
        p.inbox_activity_limit = ui.inbox_activity_limit || 25
        p.sidebar_collapse_default = ui.sidebar_collapse_default ?? false
        p.local_server_port = ui.local_server_port || 0
      },
      save: () => {
        // The theme dropdown applies live via setTheme; sync whatever is
        // current (it may also have changed from the sidebar toggle).
        this.prefs.theme = theme.mode
        const { default_category_name: _serverOwned, ...uiPrefs } = this.prefs
        // sidebar_width is an int server-side; the splitter can leave a
        // fractional px value.
        uiPrefs.sidebar_width = Math.round(uiPrefs.sidebar_width)
        return SetUIPreferences(uiPrefs)
      },
    },
    llmConfig: {
      load: async () => {
        const c = await GetLLMConfig()
        if (!c) return
        Object.assign(this.llm, {
          context: c.context || '',
          provider: c.provider || '',
          model: c.model || '',
          api_key: c.api_key || '',
          base_url: c.base_url || '',
          ai_mode: c.ai_mode || 'edit',
          min_confidence: c.min_confidence || '',
          chat_budget_mode: c.chat_budget_mode || 'warn',
          chat_budget_tokens: c.chat_budget_tokens || CHAT_BUDGET_DEFAULT,
        })
      },
      save: () => SetLLMConfig({ ...this.llm, chat_budget_tokens: clampChatBudget(this.llm.chat_budget_tokens) }),
    },
    llmProviders: {
      load: async () => {
        const [accts, routing] = await Promise.all([GetLLMAccounts(), GetLLMRouting()])
        this.llmAccounts = accts || []
        this.llmRouting = normalizeRouting(routing)
      },
      save: () => this.persistLLM(),
    },
    notify: {
      load: async () => {
        const nc = await GetNotifyConfig()
        if (!nc) return
        // system_enabled stays true so a config written from here never
        // re-introduces the old silent gate for a client that reads it.
        Object.assign(this.notify, {
          system_enabled: true,
          smtp_host: nc.smtp_host || '',
          smtp_port: nc.smtp_port || 587,
          smtp_username: nc.smtp_username || '',
          smtp_password: nc.smtp_password || '',
          smtp_from_addr: nc.smtp_from_addr || '',
          smtp_to_addr: nc.smtp_to_addr || '',
          smtp_tls: nc.smtp_tls ?? true,
          webhook_url: nc.webhook_url || '',
          webhook_auth_header: nc.webhook_auth_header || '',
        })
      },
      save: () => SetNotifyConfig(this.notify),
    },
    dueDate: {
      load: async () => {
        const dd = await GetDueDateSettings()
        if (!dd) return
        const ts = dd.thresholds || []
        const ch = (dd.channels || 'in-app,system').split(',').map((s: string) => s.trim())
        this.dueDate.enabled = dd.enabled ?? true
        this.dueDate.thresholds = { '24h': ts.includes('24h'), '1h': ts.includes('1h'), '0': ts.includes('0'), overdue: ts.includes('overdue') }
        this.dueDate.channels = { system: ch.includes('system'), email: ch.includes('email'), webhook: ch.includes('webhook') }
      },
      save: () => {
        const on = <K extends string>(m: Record<K, boolean>) => (Object.entries(m) as [K, boolean][]).filter(([, v]) => v).map(([k]) => k)
        return SaveDueDateSettings(this.dueDate.enabled, on(this.dueDate.thresholds), ['in-app', ...on(this.dueDate.channels)].join(','))
      },
    },
    capture: {
      load: async () => {
        const cp = await GetCapturePrefs()
        if (!cp) return
        this.capture = {
          videoMode: cp.videoMode ?? 'fit',
          videoBudgetMB: cp.videoBudgetMB ?? 50,
          imageMode: cp.imageMode ?? 'all',
          askMode: cp.askMode ?? 'triggers',
          triggers: {
            videoOverMB: cp.triggers?.videoOverMB ?? 50,
            galleryOverCount: cp.triggers?.galleryOverCount ?? 8,
            unsupportedUrl: cp.triggers?.unsupportedUrl ?? true,
            blocked: cp.triggers?.blocked ?? true,
            pinMayReject: cp.triggers?.pinMayReject ?? true,
          },
        }
      },
      save: () => SetCapturePrefs(this.capture),
    },
  })

  load(): Promise<void> {
    return this.sections.loadAll()
  }

  /** Saves providers + models + routing. Also called by model Test and
   *  Find models, which need the draft on the backend first. */
  persistLLM = async (): Promise<void> => {
    // Never write the placeholder arrays over providers we couldn't read.
    if (!this.sections.isReady('llmProviders')) throw new Error(t('prefs.section_not_loaded'))
    await Promise.all([SaveLLMAccounts(this.llmAccounts), SaveLLMRouting(this.llmRouting)])
  }

  /** Saves every loaded section and applies the local side effects.
   *  Returns the labels of sections that failed to save (empty = done). */
  async save(): Promise<string[]> {
    if (this.saving || this.sections.anyLoading) return []
    this.saving = true
    const { saved, failed } = await this.sections.saveReady()
    this.saving = false
    if (saved.includes('ui')) {
      setLocale(this.prefs.locale)
      nav.sidebarWidth = this.prefs.sidebar_width
      localStorage.setItem('bruv-sidebar-width', String(this.prefs.sidebar_width))
      prefsStore.typeBadgeDisplay = this.prefs.type_badge_display
    }
    // Open chats' model chips re-read the registry.
    if (saved.includes('llmProviders')) void loadLLMRegistry(true).catch(() => {})
    return failed.map(id => this.labels[id])
  }
}
