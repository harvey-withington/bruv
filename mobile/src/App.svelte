<script lang="ts">
  import { route, replace, navigate, cardURL } from './lib/router.svelte'
  import { isEnrolled, hasActiveRepo } from './lib/auth'
  import { t } from './lib/i18n.svelte'
  import EnrolPage from './routes/EnrolPage.svelte'
  import RepoPickerPage from './routes/RepoPickerPage.svelte'
  import BrowsePage from './routes/BrowsePage.svelte'
  import InboxPage from './routes/InboxPage.svelte'
  import ProjectPage from './routes/ProjectPage.svelte'
  import CategoryPage from './routes/CategoryPage.svelte'
  import CardPage from './routes/CardPage.svelte'
  import SharePage from './routes/SharePage.svelte'
  import CaptureFAB from './components/CaptureFAB.svelte'
  import ChatFAB from './components/ChatFAB.svelte'

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
      replace('/enrol')
      return
    }
    if (r.name !== 'repos' && !hasActiveRepo()) {
      replace('/repos')
    }
  })

  // The capture FAB is for *capturing while browsing* — show it on home,
  // inbox, project, and category pages. Hide on the auth/picker flows
  // (pre-enrolment-meaningless) and on Card detail (the user is already
  // editing a specific card; a "create new" button there is noise).
  const showCaptureFAB = $derived(
    route.current.name === 'home' ||
      route.current.name === 'inbox' ||
      route.current.name === 'project' ||
      route.current.name === 'category',
  )

  // The chat FAB needs a scope. Show on Card (per-card chat) and
  // Project / Category (project chat) pages — wherever an existing
  // desktop chat surface is available. Hide on Home / Inbox where
  // there's no scope available (vault-level chat doesn't exist on
  // desktop, so it doesn't ship on mobile).
  const chatScope = $derived.by(() => {
    const r = route.current
    if (r.name === 'card') return { kind: 'card' as const, cardID: r.id }
    if (r.name === 'project') return { kind: 'project' as const, brand: r.brand, stream: r.stream, project: r.project }
    if (r.name === 'category') return { kind: 'project' as const, brand: r.brand, stream: r.stream, project: r.project }
    return null
  })

  function handleCaptureSaved(cardID: string) {
    // After a quick capture, drop the user into the new card so they
    // can elaborate or move on — same intent as desktop's Inbox auto-
    // open after card creation. They can hit Back to return to where
    // they were capturing from.
    navigate(cardURL(cardID))
  }
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
{:else if route.current.name === 'category'}
  <CategoryPage
    brand={route.current.brand}
    stream={route.current.stream}
    project={route.current.project}
    category={route.current.category}
  />
{:else if route.current.name === 'card'}
  <CardPage id={route.current.id} />
{:else if route.current.name === 'share'}
  <SharePage />
{:else}
  <main class="not-found">
    <h1>{t('not_found.title')}</h1>
    <p>{t('not_found.body')}</p>
    <a href="/m/">{t('not_found.home')}</a>
  </main>
{/if}

{#if showCaptureFAB}
  <CaptureFAB onSaved={handleCaptureSaved} />
{/if}

{#if chatScope}
  <ChatFAB scope={chatScope} solo={!showCaptureFAB} />
{/if}

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
