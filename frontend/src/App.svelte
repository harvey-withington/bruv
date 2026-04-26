<script lang="ts">
  import { nav, board, prefs as prefsStore, loadCardTypes, loadGlobalTagColors, setupAgentEventListeners } from './lib/store.svelte'
  import { onMount, onDestroy } from 'svelte'
  import { loadTheme } from './lib/theme.svelte'
  import { loadNotifications, handleNewNotification } from './lib/notifications.svelte'
  import { onEvent } from './lib/events'
  import { loadLocale, t } from './lib/i18n.svelte'
  import { showToast } from './lib/toast.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import TopBar from './components/TopBar.svelte'
  import Board from './components/Board.svelte'
  import ChatSection from './components/ChatSection.svelte'
  import CardDetail from './components/CardDetail.svelte'
  import SettingsDialog from './components/SettingsDialog.svelte'
  import ProjectSettingsDialog from './components/ProjectSettingsDialog.svelte'
  import KeyboardShortcuts from './components/KeyboardShortcuts.svelte'
  import UserProfile from './components/UserProfile.svelte'
  import TagEditor from './components/TagEditor.svelte'
  import Toast from './components/Toast.svelte'
  import ConfirmDialog from './components/ConfirmDialog.svelte'
  import OptionsEditorDialog from './components/OptionsEditorDialog.svelte'
  import AboutDialog from './components/AboutDialog.svelte'
  import RepoSelector from './components/RepoSelector.svelte'
  import RemoteUnreachableScreen from './components/RemoteUnreachableScreen.svelte'
  import { loadConnections } from './lib/connections.svelte'
  import { probeBackend } from './lib/repos.svelte'
  import { resolveTransportInfo } from './lib/adapters/cloud'

  import { GetPreferences, GetCurrentRepo, GetCardLocation, GetProjectLocation, LoadProjectChatHistory, SendProjectChatMessage, ClearProjectChatHistory, ApplyProjectPendingEdits, IsLLMConfigured, MarkLLMNudgeShown } from './lib/api'

  // Restore persisted preferences
  loadTheme()
  loadLocale()

  // Restore sidebar width
  const savedWidth = localStorage.getItem('bruv-sidebar-width')
  if (savedWidth) nav.sidebarWidth = Math.max(160, Math.min(500, Number(savedWidth)))

  // Boot phase owns ONLY the boot-time transitions. Once we're
  // 'ready', nav.repoOpen is the source of truth for picker-vs-board
  // — that way closing a repo (sidebar action) naturally flips back
  // to the picker without needing an extra signal here.
  //
  //   'loading'      — initial state, before any decisions
  //   'unreachable'  — Remote active but health probe failed
  //   'ready'        — backend usable; template chooses picker vs board
  //                    based on nav.repoOpen
  let bootPhase = $state<'loading' | 'unreachable' | 'ready'>('loading')

  async function bootApp() {
    // Connection list first — required for the chip on every welcome
    // variant. Doesn't depend on RPC health (uses Wails Shell).
    await loadConnections()

    const info = await resolveTransportInfo()

    // Probe /healthz before any RPC. Cheap, sub-second on healthy
    // network; surfaces "backend down" as a friendly screen rather
    // than as a cascade of cryptic RPC errors. Loopback (Local)
    // would normally be 100% reachable, but the install-time HTTP
    // start can occasionally race the boot — a probe failure routes
    // the user to recovery instead of a stuck spinner.
    const reachable = await probeBackend()
    if (!reachable) {
      bootPhase = 'unreachable'
      return
    }

    // Per-machine prefs come from /server/rpc — works regardless of
    // whether a repo is selected, on every connection.
    let prefs: Awaited<ReturnType<typeof GetPreferences>> | undefined
    try {
      prefs = await GetPreferences()
      if (prefs?.type_badge_display) prefsStore.typeBadgeDisplay = prefs.type_badge_display
    } catch {
      // GetPreferences failed despite a successful health probe —
      // auth / RPC layer issue. Recovery screen.
      bootPhase = 'unreachable'
      return
    }

    // No repoID on the active connection → show picker. Any per-
    // repo RPC at this point would route to /repos/<empty>/rpc which
    // throws via the cloud adapter's "no repo selected" guard.
    if (!info.repoID) {
      bootPhase = 'ready'
      maybeShowLLMNudge()
      return
    }

    // reopen_last_repo=false honours the user's "always show picker"
    // preference even when a repoID happens to be on disk. The
    // recents pointer is preserved so a future toggle restores the
    // pick without forcing the user to re-select.
    if (!prefs?.reopen_last_repo) {
      bootPhase = 'ready'
      maybeShowLLMNudge()
      return
    }

    // repoID set + reopen preferred + backend reachable → fetch the
    // board. GetCurrentRepo against the resolved runtime returns the
    // canonical name + ID for the chip / nav state.
    try {
      const current = await GetCurrentRepo()
      if (current) {
        nav.repoOpen = true
        nav.repoId = current.id || current.path
        nav.repoName = current.name || ''
        loadCardTypes()
        loadGlobalTagColors()
      }
      bootPhase = 'ready'
    } catch {
      // The pointer references a repo that 404s — likely deleted
      // since last launch. Fall back to the picker rather than
      // bouncing the user to the unreachable screen for what's
      // really a stale-pointer condition.
      bootPhase = 'ready'
    } finally {
      maybeShowLLMNudge()
    }
  }
  bootApp()

  let searchCardId = $state<string | null>(null)
  let searchCardInitialTab = $state<'details' | 'agent' | undefined>(undefined)
  let resizing = $state(false)
  let showSettings = $state(false)
  let settingsInitialTab = $state<'general' | 'ai' | 'notifications' | undefined>(undefined)
  let showProjectSettings = $state(false)
  let showProfile = $state(false)
  let showTagEditor = $state(false)
  let showProjectChat = $state(false)
  // Project-chat DOM-lag flag — see CardDetail's chatInDom for the same
  // pattern. ChatSection's slideOutWidth transition lives on its own
  // inner `{#if visible}` element; if the OUTER {#if} here unmounts the
  // component synchronously when the user closes the panel, the out
  // transition never runs. Keeping projectChatInDom true for the
  // transition duration lets the slide-out animation play before the
  // component is torn down.
  const PROJECT_CHAT_OUT_DURATION = 240
  // svelte-ignore state_referenced_locally
  let projectChatInDom = $state(showProjectChat)
  $effect(() => {
    if (showProjectChat) {
      projectChatInDom = true
    } else if (projectChatInDom) {
      const timer = setTimeout(() => { projectChatInDom = false }, PROJECT_CHAT_OUT_DURATION)
      return () => clearTimeout(timer)
    }
  })
  let showKeyboardShortcuts = $state(false)
  let showAbout = $state(false)
  let showConnections = $state(false)

  // First-run LLM nudge: fire once per install, after the initial prefs load.
  // When the user has no LLM account configured, open the Settings dialog
  // straight on the AI tab so they can add one. We mark the flag immediately
  // so a crash before the user interacts doesn't re-nudge next launch.
  async function maybeShowLLMNudge() {
    try {
      const p = await GetPreferences() as { llm_nudge_shown?: boolean } | null
      if (p?.llm_nudge_shown) return
      const configured = await IsLLMConfigured()
      if (configured) return
      await MarkLLMNudgeShown()
      settingsInitialTab = 'ai'
      showSettings = true
    } catch { /* non-critical */ }
  }

  function handleSearchSelectCard(cardId: string, tab?: 'details' | 'agent') {
    searchCardInitialTab = tab
    searchCardId = cardId
  }

  // When opening a card via navigation, resolve its category from the loaded board
  let searchCategoryId = $state<string | null>(null)
  let searchCategoryName = $state<string | null>(null)

  function findCardInBoard(cardId: string): { id: string; name: string } | null {
    for (const cat of board.categories) {
      if (cat.cards?.some((c: { id: string }) => c.id === cardId)) {
        return { id: cat.id, name: cat.name }
      }
    }
    return null
  }

  // Listen for internal bruv: link navigation events
  onMount(() => {
    async function handleBruvNav(e: Event) {
      const detail = (e as CustomEvent).detail
      if (detail.type === 'card') {
        // Close ALL open card dialogs (both search-opened and board-opened)
        searchCardId = null
        searchCategoryId = null
        searchCategoryName = null
        document.dispatchEvent(new CustomEvent('bruv:close-card-detail'))
        try {
          const loc = await GetCardLocation(detail.id)
          if (loc) {
            // Ask sidebar to select this project (which loads the board) and wait for it
            await new Promise<void>(resolve => {
              document.dispatchEvent(new CustomEvent('bruv:select-project', { detail: { ...loc, resolve } }))
            })
          }
        } catch { /* card may be unpinned — just open it without switching */ }

        // Resolve category context from the loaded board
        const found = findCardInBoard(detail.id)
        if (found) {
          searchCategoryId = found.id
          searchCategoryName = found.name
        }

        searchCardId = detail.id
      } else if (detail.type === 'project') {
        searchCardId = null
        document.dispatchEvent(new CustomEvent('bruv:close-card-detail'))
        try {
          const loc = await GetProjectLocation(detail.id)
          if (loc) {
            await new Promise<void>(resolve => {
              document.dispatchEvent(new CustomEvent('bruv:select-project', { detail: { ...loc, resolve } }))
            })
          }
        } catch (e) { console.error('Failed to navigate to project', e) }
      }
    }
    document.addEventListener('bruv:navigate', handleBruvNav)
    return () => document.removeEventListener('bruv:navigate', handleBruvNav)
  })

  // Global keyboard shortcuts (only when no input/textarea focused)
  function isInputFocused(): boolean {
    const el = document.activeElement
    if (!el) return false
    const tag = el.tagName.toLowerCase()
    return tag === 'input' || tag === 'textarea' || (el as HTMLElement).isContentEditable
  }

  onMount(() => {
    function handleGlobalKeydown(e: KeyboardEvent) {
      if (isInputFocused()) return
      if (e.key === '?' && !e.ctrlKey && !e.metaKey) {
        e.preventDefault()
        showKeyboardShortcuts = !showKeyboardShortcuts
      } else if (e.key === '/' && !e.ctrlKey && !e.metaKey) {
        e.preventDefault()
        document.querySelector<HTMLInputElement>('.search-box input, .search-input')?.focus()
      } else if (e.key === 'p' && !e.ctrlKey && !e.metaKey && nav.projectSlug) {
        e.preventDefault()
        showProjectChat = !showProjectChat
      }
    }
    window.addEventListener('keydown', handleGlobalKeydown)
    return () => window.removeEventListener('keydown', handleGlobalKeydown)
  })

  // Notification system (Wails events + persistent store)
  let notifCleanups: (() => void)[] = []
  // One-shot breadcrumb so the user finds out their search index is
  // stale without being spammed if multiple index writes fail in a row.
  // The backend emits "index:stale" from logIdxErr whenever an index
  // operation returns an error. Clearing the flag on rebuild is handled
  // by the Rebuild button in AboutDialog (via the success toast) — a
  // soft-reset fine for alpha; per-session re-warn is enough.
  let indexStaleNotified = false
  onMount(() => {
    // Notifications are per-repo (and on Remote multi-repo, only
    // reachable via /repos/<id>/rpc). Skip the initial fetch when
    // we're on the picker — onMount won't re-fire on its own, but
    // any post-pick repo switch reloads the page (the established
    // pattern), so the next mount lands with nav.repoOpen=true and
    // this guard passes.
    if (nav.repoOpen) loadNotifications()
    notifCleanups = [
      onEvent('notification:new', (data: any) => {
        handleNewNotification(data)
      }),
      onEvent('index:stale', () => {
        if (indexStaleNotified) return
        indexStaleNotified = true
        showToast(t('about.index_stale'), 'error')
      }),
      setupAgentEventListeners(),
    ]
  })
  onDestroy(() => { for (const fn of notifCleanups) fn?.() })

  // Declared as MouseEvent because Svelte 5's `onpointerdown` attribute
  // type on HTMLElement is MouseEventHandler — PointerEvent extends
  // MouseEvent at runtime so we can safely narrow inside.
  function onSplitterDown(e: MouseEvent) {
    if (nav.sidebarCollapsed) return
    e.preventDefault()
    resizing = true
    const startX = e.clientX
    const startW = nav.sidebarWidth
    // Capture the pointer on the splitter element so move/up events
    // keep firing even if the cursor leaves the splitter — essential
    // for drag reliability on both mouse and touch.
    const pe = e as PointerEvent
    const target = e.currentTarget as HTMLElement | null
    const pointerId = pe.pointerId
    target?.setPointerCapture?.(pointerId)

    function onMove(ev: PointerEvent) {
      const delta = ev.clientX - startX
      nav.sidebarWidth = Math.max(160, Math.min(500, startW + delta))
    }

    function onUp() {
      resizing = false
      localStorage.setItem('bruv-sidebar-width', String(nav.sidebarWidth))
      try { target?.releasePointerCapture?.(pointerId) } catch { /* already released */ }
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }
</script>

{#if bootPhase === 'loading'}
  <div class="loading-screen">
    <span class="loading-text">{t('app.starting')}</span>
  </div>
{:else if bootPhase === 'unreachable'}
  <RemoteUnreachableScreen
    onOpenConnections={() => showConnections = true}
    onProbeOk={() => bootApp()}
  />
{:else if !nav.repoOpen}
  <!-- bootPhase is 'ready' but no repo is open — first-launch
       picker, post-boot close-repo, etc. all funnel here. -->
  <RepoSelector mode="fullscreen" />
{:else}
  <div class="app-shell" class:resizing>
    <Sidebar
      onOpenPrefs={() => { settingsInitialTab = undefined; showSettings = true }}
      onOpenProfile={() => showProfile = true}
      onOpenAbout={() => showAbout = true}
      onOpenConnections={() => showConnections = true}
    />
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="splitter"
      role="separator"
      tabindex="-1"
      class:collapsed={nav.sidebarCollapsed}
      onpointerdown={onSplitterDown}
    ></div>
    <div class="main-area">
      <TopBar
        onSelectCard={handleSearchSelectCard}
        onOpenTagEditor={() => showTagEditor = true}
        onOpenProjectSettings={() => showProjectSettings = true}
        onToggleProjectChat={() => showProjectChat = !showProjectChat}
        projectChatActive={showProjectChat}
      />
      <div class="board-row">
        <Board />
        {#if projectChatInDom && nav.projectSlug}
          <ChatSection
            cardId=""
            visible={showProjectChat}
            projectMode={true}
            reloadKey={nav.projectSlug}
            loadFn={() => LoadProjectChatHistory(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!)}
            sendFn={(text, contextLevel) => SendProjectChatMessage(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!, text, contextLevel ?? 'all')}
            clearFn={() => ClearProjectChatHistory(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!)}
            applyFn={(msgID, acceptIDs) => ApplyProjectPendingEdits(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!, msgID, acceptIDs)}
          />
        {/if}
      </div>
    </div>
  </div>

  {#if searchCardId}
    <CardDetail
      cardId={searchCardId}
      currentCategoryId={searchCategoryId}
      currentCategoryName={searchCategoryName}
      initialTab={searchCardInitialTab}
      onClose={() => { searchCardId = null; searchCategoryId = null; searchCategoryName = null; searchCardInitialTab = undefined }}
    />
  {/if}

  {#if showSettings}
    <SettingsDialog onClose={() => { showSettings = false; settingsInitialTab = undefined }} initialTab={settingsInitialTab} />
  {/if}

  {#if showProjectSettings}
    <ProjectSettingsDialog onClose={() => showProjectSettings = false} />
  {/if}

  {#if showProfile}
    <UserProfile onClose={() => showProfile = false} />
  {/if}

  {#if showTagEditor}
    <TagEditor onClose={() => showTagEditor = false} />
  {/if}
{/if}

{#if showConnections}
  <!-- Same RepoSelector component, dialog mode. Mounted outside the
       repo-open branch so it's reachable from every state — including
       the fullscreen picker, where clicking the chip-style "switch"
       call-to-action opens it as a modal on top. -->
  <RepoSelector mode="dialog" onClose={() => { showConnections = false }} />
{/if}

{#if showKeyboardShortcuts}
  <KeyboardShortcuts onClose={() => showKeyboardShortcuts = false} />
{/if}

{#if showAbout}
  <AboutDialog onClose={() => showAbout = false} />
{/if}

<Toast />
<ConfirmDialog />
<OptionsEditorDialog />

<style>
  .app-shell {
    display: flex;
    height: 100vh;
    overflow: hidden;
  }

  .app-shell.resizing {
    cursor: col-resize;
    user-select: none;
  }

  .splitter {
    width: 4px;
    cursor: col-resize;
    background: transparent;
    transition: background var(--duration-normal) var(--ease-out);
    flex-shrink: 0;
    position: relative;
    z-index: 10;
    margin-left: -2px;
    margin-right: -2px;
  }

  .splitter::before {
    content: '';
    position: absolute;
    inset: 0 -3px;
  }

  .splitter:hover,
  .app-shell.resizing .splitter {
    background: var(--accent);
    box-shadow: 0 0 6px var(--accent-glow-1);
  }

  .splitter.collapsed {
    cursor: default;
    pointer-events: none;
  }

  .main-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .board-row {
    flex: 1;
    display: flex;
    overflow: hidden;
  }

  .loading-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    background: var(--bg-base);
    animation: fade-in var(--duration-slow) var(--ease-out);
  }

  .loading-text {
    color: var(--text-muted);
    font-size: 0.9rem;
    letter-spacing: 0.05em;
    animation: fade-in-up 0.6s var(--ease-out) both;
  }
</style>
