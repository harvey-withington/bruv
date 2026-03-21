<script lang="ts">
  import { nav } from './lib/store.svelte'
  import { onMount } from 'svelte'
  import { loadTheme } from './lib/theme.svelte'
  import { loadLocale, t } from './lib/i18n.svelte'
  import WelcomeScreen from './components/WelcomeScreen.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import TopBar from './components/TopBar.svelte'
  import Board from './components/Board.svelte'
  import CardDetail from './components/CardDetail.svelte'
  import UserPreferences from './components/UserPreferences.svelte'
  import UserProfile from './components/UserProfile.svelte'
  import LLMSettings from './components/LLMSettings.svelte'

  import { GetPreferences, ListRecentRepos, OpenRepository, GetCardLocation, GetProjectLocation } from './lib/api'

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
      const prefs = await GetPreferences()
      if (!prefs?.reopen_last_repo) return
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

  let searchCardId = $state<string | null>(null)
  let resizing = $state(false)
  let showPrefs = $state(false)
  let showProfile = $state(false)
  let showLLMSettings = $state(false)

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
      onOpenPrefs={() => showPrefs = true}
      onOpenProfile={() => showProfile = true}
      onOpenLLMSettings={() => showLLMSettings = true}
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
        onOpenLabels={() => { /* TODO: open labels dialog */ }}
        onOpenProjectSettings={() => { /* TODO: open project settings */ }}
        onCreateAIChat={() => { /* TODO: create AI chat card */ }}
      />
      <Board />
    </div>
  </div>

  {#if searchCardId}
    <CardDetail
      cardId={searchCardId}
      onClose={() => searchCardId = null}
    />
  {/if}

  {#if showPrefs}
    <UserPreferences onClose={() => showPrefs = false} />
  {/if}

  {#if showProfile}
    <UserProfile onClose={() => showProfile = false} />
  {/if}

  {#if showLLMSettings}
    <LLMSettings onClose={() => showLLMSettings = false} />
  {/if}
{:else}
  <WelcomeScreen />
{/if}

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
