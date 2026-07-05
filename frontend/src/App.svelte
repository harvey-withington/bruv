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
  import SidePanel from './components/SidePanel.svelte'
  import WorkspacePanel from './components/workspace/WorkspacePanel.svelte'
  import CardDetail from './components/CardDetail.svelte'
  import { BotMessageSquare, Briefcase } from 'lucide-svelte'
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
  import ConnectionOverlay from './components/ConnectionOverlay.svelte'
  import { loadConnections } from './lib/connections.svelte'
  import { probeBackend } from './lib/repos.svelte'
  import { installResilience } from './lib/connectivity.svelte'
  import { resolveTransportInfo } from '@shared/adapters/cloud'

  import { GetUIPreferences, SetUIPreferences, GetCurrentRepo, GetCardLocation, GetProjectLocation, LoadProjectChatHistory, SendProjectChatMessage, ClearProjectChatHistory, ApplyProjectPendingEdits, IsLLMConfigured } from '@shared/api'

  // Restore persisted preferences
  loadTheme()
  loadLocale()

  // Install runtime connection-loss handling before any RPC. Only arms
  // for remote connections — the local backend can't drop.
  installResilience()

  // Restore sidebar width
  const savedWidth = localStorage.getItem('bruv-sidebar-width')
  if (savedWidth) nav.sidebarWidth = Math.round(Math.max(160, Math.min(500, Number(savedWidth))))

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

    // Per-DEVICE prefs come from the local shell (or localStorage in
    // browser mode) — never over RPC, so each machine keeps its own
    // theme/layout even against a remote backend. A failure here is
    // local-disk trouble, not connectivity — don't route to recovery.
    let uiPrefs: Awaited<ReturnType<typeof GetUIPreferences>> | undefined
    try {
      uiPrefs = await GetUIPreferences()
      if (uiPrefs?.type_badge_display) prefsStore.typeBadgeDisplay = uiPrefs.type_badge_display
    } catch { /* defaults apply */ }

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
    if (!uiPrefs?.reopen_last_repo) {
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
  // Right-hand side panel: one resizable container with bottom tabs
  // (VS Code-style) hosting Project Chat + Workspace. The TopBar buttons
  // open the panel focused on their tab, or close it when their tab is
  // already frontmost.
  type SideTabID = 'chat' | 'workspace'
  // Open state + active tab survive refresh (localStorage) — an open panel
  // is part of the user's arranged workspace, not transient chrome.
  const SIDE_PANEL_OPEN_KEY = 'bruv:sidePanelOpen'
  const SIDE_PANEL_TAB_KEY = 'bruv:sidePanelTab'
  let sidePanelOpen = $state(localStorage.getItem(SIDE_PANEL_OPEN_KEY) === '1')
  let sidePanelTab = $state<SideTabID>(localStorage.getItem(SIDE_PANEL_TAB_KEY) === 'workspace' ? 'workspace' : 'chat')
  $effect(() => {
    localStorage.setItem(SIDE_PANEL_OPEN_KEY, sidePanelOpen ? '1' : '0')
    localStorage.setItem(SIDE_PANEL_TAB_KEY, sidePanelTab)
  })
  let workspaceFileRequest = $state<{ wsId: string; path: string } | null>(null)
  const sideTabs = $derived([
    // Labels intentionally match the panes' own header titles.
    { id: 'chat', label: t('chat.project_title'), icon: BotMessageSquare },
    { id: 'workspace', label: t('workspace.title'), icon: Briefcase },
  ])
  function toggleSideTab(tab: SideTabID) {
    if (sidePanelOpen && sidePanelTab === tab) {
      sidePanelOpen = false
    } else {
      sidePanelTab = tab
      sidePanelOpen = true
    }
  }
  // Panel DOM-lag flag — see CardDetail's chatInDom for the same pattern.
  // SidePanel's slideOutWidth transition lives on its own inner
  // `{#if visible}` element; if the OUTER {#if} here unmounts the
  // component synchronously on close, the out transition never runs.
  const SIDE_PANEL_OUT_DURATION = 240
  // svelte-ignore state_referenced_locally
  let sidePanelInDom = $state(sidePanelOpen)
  $effect(() => {
    if (sidePanelOpen) {
      sidePanelInDom = true
    } else if (sidePanelInDom) {
      const timer = setTimeout(() => { sidePanelInDom = false }, SIDE_PANEL_OUT_DURATION)
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
      const p = await GetUIPreferences()
      if (p.llm_nudge_shown) return
      const configured = await IsLLMConfigured()
      if (configured) return
      await SetUIPreferences({ llm_nudge_shown: true })
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
        } catch (e) {
          console.error('Failed to navigate to project', e)
          showToast(t('error.navigate_failed'), 'error')
        }
      } else if (detail.type === 'workspace-file') {
        // workspace://<ws-id>/<path> — resolve inside the current project's
        // workspace via the panel (v1: links work within their own project).
        const sep = detail.id.indexOf('/')
        workspaceFileRequest = {
          wsId: sep === -1 ? detail.id : detail.id.slice(0, sep),
          path: sep === -1 ? '' : detail.id.slice(sep + 1),
        }
        sidePanelTab = 'workspace'
        sidePanelOpen = true
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
        toggleSideTab('chat')
      } else if (e.key === 'w' && !e.ctrlKey && !e.metaKey && nav.projectSlug) {
        e.preventDefault()
        toggleSideTab('workspace')
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
    // Notifications are per-machine (MachineService.GetNotifications
    // routes via /server/rpc, no repoID needed) so we can fetch
    // unconditionally — works on the picker too. The previous
    // nav.repoOpen gate was a leftover from when GetNotifications
    // was per-repo; with that gate, the boot race between bootApp
    // (sets nav.repoOpen) and onMount could swallow the initial load
    // and the user landed with an empty notification list until the
    // next event arrived.
    loadNotifications()
    notifCleanups = [
      onEvent('notification:new', (data: any) => {
        handleNewNotification(data)
      }),
      // Belt-and-braces: refresh from disk after every agent run so
      // any in-app notification fired by the agent shows up even if
      // the live notification:new event missed us (mid-reload, SSE
      // not yet reconnected, etc.). Cheap — just reads the persisted
      // list from /server/rpc → MachineService → notifications.json.
      onEvent('agent:completed', () => loadNotifications()),
      onEvent('agent:failed', () => loadNotifications()),
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
      nav.sidebarWidth = Math.round(Math.max(160, Math.min(500, startW + delta)))
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
        onToggleProjectChat={() => toggleSideTab('chat')}
        projectChatActive={sidePanelOpen && sidePanelTab === 'chat'}
        onToggleWorkspace={() => toggleSideTab('workspace')}
        workspaceActive={sidePanelOpen && sidePanelTab === 'workspace'}
      />
      <div class="board-row">
        <Board />
        {#if sidePanelInDom && nav.projectSlug}
          <SidePanel
            visible={sidePanelOpen}
            tabs={sideTabs}
            bind:activeTab={sidePanelTab}
            onClose={() => sidePanelOpen = false}
          >
            {#snippet children(active: string)}
              <!-- Both panes stay mounted; inactive ones hide via CSS so
                   chat state (draft, scroll, history) survives tab flips. -->
              <div class="sp-tab-pane" class:pane-hidden={active !== 'chat'}>
                <ChatSection
                  cardId=""
                  hosted
                  visible={sidePanelOpen}
                  projectMode={true}
                  reloadKey={nav.projectSlug ?? undefined}
                  loadFn={() => LoadProjectChatHistory(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!)}
                  sendFn={(text, contextLevel) => SendProjectChatMessage(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!, text, contextLevel ?? 'all')}
                  clearFn={() => ClearProjectChatHistory(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!)}
                  applyFn={(msgID, acceptIDs) => ApplyProjectPendingEdits(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!, msgID, acceptIDs)}
                />
              </div>
              <div class="sp-tab-pane" class:pane-hidden={active !== 'workspace'}>
                <WorkspacePanel
                  brandSlug={nav.brandSlug!}
                  streamSlug={nav.streamSlug!}
                  projectSlug={nav.projectSlug!}
                  openRequest={workspaceFileRequest}
                  onRequestHandled={() => workspaceFileRequest = null}
                />
              </div>
            {/snippet}
          </SidePanel>
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
<ConnectionOverlay />

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

  /* Side-panel tab panes: both stay mounted (chat keeps its state across
     tab flips); the inactive one hides. Rendered inside SidePanel via the
     snippet, so the styles live here where the markup is authored. */
  .sp-tab-pane {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }
  .sp-tab-pane.pane-hidden {
    display: none;
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
