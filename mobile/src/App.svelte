<script lang="ts">
  import { route, replace } from './lib/router.svelte'
  import { isEnrolled, hasActiveRepo } from './lib/auth'
  import { startEvents, stopEvents } from './lib/events.svelte'
  import { t } from './lib/i18n.svelte'
  import EnrolPage from './routes/EnrolPage.svelte'
  import RepoPickerPage from './routes/RepoPickerPage.svelte'
  import BrowsePage from './routes/BrowsePage.svelte'
  import InboxPage from './routes/InboxPage.svelte'
  import ProjectPage from './routes/ProjectPage.svelte'
  import CardPage from './routes/CardPage.svelte'
  import SharePage from './routes/SharePage.svelte'
  import SettingsPage from './routes/SettingsPage.svelte'
  import ActivityPage from './routes/ActivityPage.svelte'
  import Toast from './components/Toast.svelte'
  import ConnectionOverlay from './components/ConnectionOverlay.svelte'

  // Two-stage auth gate, runs reactively on every route change:
  //
  //  1. Not enrolled → /enrol  (everything else needs a device token).
  //  2. Enrolled but no repo picked → /repos  (every per-repo route
  //     needs a repo ID in the URL; nothing useful renders without one).
  //
  // Order matters: enrol gate runs before repo gate so a back gesture
  // from /repos onto / on an unenrolled device redirects all the way
  // to /enrol, not into a half-broken state.
  $effect(() => {
    const r = route.current
    if (r.name === 'enrol') return
    if (!isEnrolled()) {
      stopEvents()
      replace('/enrol')
      return
    }
    if (r.name !== 'repos' && !hasActiveRepo()) {
      stopEvents()
      replace('/repos')
      return
    }
    if (hasActiveRepo()) {
      // Idempotent — startEvents() bails if a stream is already
      // attached to the same repo.
      startEvents()
    }
  })

  // Quick capture and AI chat entry points live in each page's topbar
  // (CaptureButton / ChatButton) — Browse/Inbox/Project get capture,
  // Card/Project get chat. No app-level floating buttons: they don't
  // suit small screens and overlaid page content.
</script>

{#if route.current.name === 'enrol'}
  <EnrolPage />
{:else if route.current.name === 'repos'}
  <RepoPickerPage />
{:else if route.current.name === 'home'}
  <BrowsePage />
{:else if route.current.name === 'inbox'}
  <InboxPage />
{:else if route.current.name === 'project'}
  <ProjectPage
    brand={route.current.brand}
    stream={route.current.stream}
    project={route.current.project}
  />
{:else if route.current.name === 'card'}
  <CardPage id={route.current.id} />
{:else if route.current.name === 'share'}
  <SharePage />
{:else if route.current.name === 'settings'}
  <SettingsPage />
{:else if route.current.name === 'activity'}
  <ActivityPage />
{:else}
  <main class="not-found">
    <h1>{t('not_found.title')}</h1>
    <p>{t('not_found.body')}</p>
    <a href="/m/">{t('not_found.home')}</a>
  </main>
{/if}

<Toast />
<ConnectionOverlay />

<style>
  .not-found {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    text-align: center;
    padding: 2rem;
  }

  .not-found h1 {
    margin: 0 0 0.5rem;
    color: var(--text);
  }

  .not-found p {
    color: var(--text-muted);
    margin: 0 0 1rem;
  }

  .not-found a {
    color: var(--accent);
  }
</style>
