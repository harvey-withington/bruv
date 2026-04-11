<script lang="ts">
  import { nav, board, prefs as prefsStore, loadCardTypes, loadGlobalTagColors, setupAgentEventListeners } from './lib/store.svelte'
  import { onMount, onDestroy } from 'svelte'
  import { loadTheme } from './lib/theme.svelte'
  import { loadNotifications, handleNewNotification } from './lib/notifications.svelte'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import { loadLocale, t } from './lib/i18n.svelte'
  import WelcomeScreen from './components/WelcomeScreen.svelte'
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

  import { GetPreferences, ListRecentRepos, OpenRepository, GetCardLocation, GetProjectLocation, LoadProjectChatHistory, SendProjectChatMessage, ClearProjectChatHistory, ApplyProjectPendingEdits, IsLLMConfigured, MarkLLMNudgeShown } from './lib/api'

  // Restore persisted preferences
  loadTheme()
  loadLocale()

  // Restore sidebar width
  const savedWidth = localStorage.getItem('bruv-sidebar-width')
  if (savedWidth) nav.sidebarWidth = Math.max(160, Math.min(500, Number(savedWidth)))

  // Auto-reopen last repo if preference is enabled
  let appLoading = $state(true)

  async function tryReopenLastRepo() {
    try {
      const p = await GetPreferences()
      if (p?.type_badge_display) prefsStore.typeBadgeDisplay = p.type_badge_display
      if (!p?.reopen_last_repo) return
      const recent = await ListRecentRepos()
      if (!recent?.length) return
      const last = recent[0]
      await OpenRepository(last.path)
      nav.repoOpen = true
      nav.repoId = last.path
    } catch { /* silently fall back to welcome screen */ }
    finally {
      appLoading = false
      maybeShowLLMNudge()
    }
  }
  tryReopenLastRepo()
  loadCardTypes()
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
  let showKeyboardShortcuts = $state(false)
  let showAbout = $state(false)

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
  onMount(() => {
    loadNotifications()
    notifCleanups = [
      EventsOn('notification:new', (data: any) => {
        handleNewNotification(data)
      }),
      setupAgentEventListeners(),
    ]
  })
  onDestroy(() => { for (const fn of notifCleanups) fn?.() })

  function onSplitterDown(e: MouseEvent) {
    if (nav.sidebarCollapsed) return
    e.preventDefault()
    resizing = true
    const startX = e.clientX
    const startW = nav.sidebarWidth

    function onMove(ev: MouseEvent) {
      const delta = ev.clientX - startX
      nav.sidebarWidth = Math.max(160, Math.min(500, startW + delta))
    }

    function onUp() {
      resizing = false
      localStorage.setItem('bruv-sidebar-width', String(nav.sidebarWidth))
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }

    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }
</script>

{#if appLoading}
  <div class="loading-screen">
    <span class="loading-text">{t('app.starting')}</span>
  </div>
{:else if nav.repoOpen}
  <div class="app-shell" class:resizing>
    <Sidebar
      onOpenPrefs={() => { settingsInitialTab = undefined; showSettings = true }}
      onOpenProfile={() => showProfile = true}
      onOpenAbout={() => showAbout = true}
    />
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="splitter"
      role="separator"
      tabindex="-1"
      class:collapsed={nav.sidebarCollapsed}
      onmousedown={onSplitterDown}
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
        {#if showProjectChat && nav.projectSlug}
          <ChatSection
            cardId=""
            visible={true}
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

{:else}
  <WelcomeScreen />
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
    transition: background 0.15s;
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
  }

  .loading-text {
    color: var(--text-muted);
    font-size: 0.9rem;
    letter-spacing: 0.05em;
  }
</style>
