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

  import { GetPreferences, GetLastOpenedLocalRepoPath, OpenRepository, GetCurrentRepo, GetCardLocation, GetProjectLocation, LoadProjectChatHistory, SendProjectChatMessage, ClearProjectChatHistory, ApplyProjectPendingEdits, IsLLMConfigured, MarkLLMNudgeShown } from './lib/api'

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
    const isRemote = info.remote === 'true'

    // Remote without a repoID set means the user hasn't picked yet
    // (or just removed the active connection's saved repo) — show
    // the picker before any RPC fires (those URLs would 404 without
    // the /repos/<id>/ prefix).
    if (isRemote && !info.repoID) {
      bootPhase = 'ready'
      return
    }

    // Remote WITH a repoID: probe /healthz before trusting the
    // adapter. Cheap, sub-second on a healthy tailnet, surfaces
    // "server is down" as a friendly screen rather than as 30
    // cryptic RPC errors.
    if (isRemote) {
      const reachable = await probeBackend()
      if (!reachable) {
        bootPhase = 'unreachable'
        return
      }
    }

    // Backend is reachable (or we're Local — loopback is always
    // reachable). Try to load preferences + auto-reopen.
    try {
      const p = await GetPreferences()
      if (p?.type_badge_display) prefsStore.typeBadgeDisplay = p.type_badge_display

      // GetCurrentRepo: Remote always has its installed repo open;
      // Local returns null until the user has picked / opened one.
      try {
        const current = await GetCurrentRepo()
        if (current) {
          nav.repoOpen = true
          nav.repoId = current.id || current.path
          bootPhase = 'ready'
          return
        }
      } catch { /* fall through */ }

      // Local fallback: auto-reopen last repo if preference is on.
      // Reads the per-machine "last opened on Local" pointer that
      // App.OpenRepository stamps into repo-recents (key ""). The
      // path is resolved via the registry — if the entry was
      // removed, the call returns "" and we fall through.
      if (p?.reopen_last_repo) {
        const lastPath = await GetLastOpenedLocalRepoPath()
        if (lastPath) {
          try {
            await OpenRepository(lastPath)
            nav.repoOpen = true
            nav.repoId = lastPath
            bootPhase = 'ready'
            return
          } catch { /* path may have moved; fall through to picker */ }
        }
      }

      // Nothing auto-loaded — show the picker.
      bootPhase = 'ready'
    } catch {
      // GetPreferences failed despite a successful health probe —
      // probably an auth / RPC layer issue. Treat as unreachable so
      // the user gets recovery actions rather than a stuck spinner.
      bootPhase = 'unreachable'
    } finally {
      // Card types are repo-scoped — only fetch once a repo is open.
      if (nav.repoOpen) loadCardTypes()
      maybeShowLLMNudge()
    }
  }
  bootApp()
  loadGlobalTagColors()

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
    loadNotifications()
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
