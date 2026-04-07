<script lang="ts">
  import { nav, prefs as prefsStore, loadCardTypes, loadGlobalTagColors } from './lib/store.svelte'
  import { onMount } from 'svelte'
  import { loadTheme } from './lib/theme.svelte'
  import { loadLocale, t } from './lib/i18n.svelte'
  import WelcomeScreen from './components/WelcomeScreen.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import TopBar from './components/TopBar.svelte'
  import Board from './components/Board.svelte'
  import ChatSection from './components/ChatSection.svelte'
  import CardDetail from './components/CardDetail.svelte'
  import SettingsDialog from './components/SettingsDialog.svelte'
  import UserProfile from './components/UserProfile.svelte'
  import TagEditor from './components/TagEditor.svelte'
  import Toast from './components/Toast.svelte'
  import ConfirmDialog from './components/ConfirmDialog.svelte'

  import { GetPreferences, ListRecentRepos, OpenRepository, GetCardLocation, GetProjectLocation, LoadProjectChatHistory, SendProjectChatMessage } from './lib/api'

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
    finally { appLoading = false }
  }
  tryReopenLastRepo()
  loadCardTypes()
  loadGlobalTagColors()

  let searchCardId = $state<string | null>(null)
  let resizing = $state(false)
  let showSettings = $state(false)
  let showProfile = $state(false)
  let showTagEditor = $state(false)
  let showProjectChat = $state(false)

  function handleSearchSelectCard(cardId: string) {
    searchCardId = cardId
  }

  // Listen for internal bruv: link navigation events
  onMount(() => {
    async function handleBruvNav(e: Event) {
      const detail = (e as CustomEvent).detail
      if (detail.type === 'card') {
        // Close ALL open card dialogs (both search-opened and board-opened)
        searchCardId = null
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
      onOpenPrefs={() => showSettings = true}
      onOpenProfile={() => showProfile = true}
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
        onOpenProjectSettings={() => { /* TODO: open project settings */ }}
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
            sendFn={(text) => SendProjectChatMessage(nav.brandSlug!, nav.streamSlug!, nav.projectSlug!, text)}
          />
        {/if}
      </div>
    </div>
  </div>

  {#if searchCardId}
    <CardDetail
      cardId={searchCardId}
      onClose={() => searchCardId = null}
    />
  {/if}

  {#if showSettings}
    <SettingsDialog onClose={() => showSettings = false} />
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

<Toast />
<ConfirmDialog />

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
