<script lang="ts">
  // A card's chat on the UNCLI engine (opted into in the desktop's
  // Settings → AI): UNCLI's <uncli-chat> in the chat sheet, showing the
  // card's session (shared/uncliChat.openCardSession), in its compact
  // layout by its width. The element shows UNCLI's own download and
  // sign-in steps until its CLI is ready, and approvals inline. BRUV
  // supplies its sign-in, @mentions of cards, bruv: links and the open
  // card as each turn's context (cardChatHost).
  import { onMount, untrack } from 'svelte'
  import { errorDetail } from '@shared/errors'
  import { cardChatHost, openCardSession, uncliSrc, UNCLI_LINK_SCHEMES, type UncliChatElement } from '@shared/uncliChat'
  import type { SearchResult } from '@shared/types'
  import { readEnrolment, repoRPC } from '../../lib/auth'
  import { replace } from '../../lib/router.svelte'
  import { followBruv } from '../../lib/mentions.svelte'
  import { theme } from '../../lib/theme.svelte'
  import { t } from '../../lib/i18n.svelte'
  import ChatSheetShell from './ChatSheetShell.svelte'

  let {
    cardID,
    card,
    onClose,
  }: {
    cardID: string
    /** The open card, for each turn's context. */
    card: () => { id: string; title: string; type: string } | null
    onClose: () => void
  } = $props()

  let src = $state('')
  let token = ''
  let error = $state('')
  let node = $state<HTMLElement | null>(null)
  // Set when a link navigates: replace() consumes the sheet's history
  // entry (ChatSheetShell.navigatingAway).
  let navigatedAway = false

  // The element (about 200 KB) loads when a chat first opens.
  onMount(() => {
    const enrolment = readEnrolment()
    if (!enrolment) {
      error = t('uncli.load_failed', { detail: t('uncli.not_paired') })
      return
    }
    token = enrolment.deviceToken
    import('@uncli/chat/element')
      .then(() => { src = uncliSrc(enrolment.serverURL) })
      .catch(e => { error = t('uncli.load_failed', { detail: errorDetail(e) }) })
  })

  function follow(target: string) {
    navigatedAway = true
    onClose()
    followBruv(target, replace)
  }

  // The host before src: the element connects as soon as it has a src,
  // and its first requests must already carry BRUV's sign-in. Then the
  // card's session.
  $effect(() => {
    const el = node as UncliChatElement | null
    const id = cardID
    if (!el || !src) return
    untrack(() => {
      el.host = cardChatHost({
        token: () => token,
        follow,
        searchCards: q => repoRPC<SearchResult[]>('SearchCards', [q, 8]),
        card,
        cardLabel: t('uncli.mention_card'),
      })
      el.setAttribute('src', src)
      openCardSession(el, id, {
        get: cardId => repoRPC<string>('GetCardUncliSession', [cardId]),
        set: (cardId, session) => repoRPC<void>('SetCardUncliSession', [cardId, session]),
      }).catch(e => {
        error = t('uncli.session_failed', { detail: errorDetail(e) })
      })
    })
  })

  // Escape closes the sheet, unless it's for the chat itself (its own
  // menus and dialogs, inside its shadow root).
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape' || (node && e.composedPath().includes(node))) return
    e.preventDefault()
    e.stopPropagation()
    onClose()
  }
</script>

<ChatSheetShell title={t('chat.title')} note={t('uncli.engine_badge')} {onClose} {onKeydown} navigatingAway={() => navigatedAway}>
  {#if error}
    <p class="status error" role="alert">{error}</p>
  {:else if src}
    <!-- src is set by the effect above, after the host. -->
    <uncli-chat bind:this={node} scheme={theme.current === 'auto' ? undefined : theme.current} link-schemes={UNCLI_LINK_SCHEMES} storage-key="bruv-card-chat"></uncli-chat>
  {:else}
    <p class="status">{t('chat.loading')}</p>
  {/if}
</ChatSheetShell>

<style>
  uncli-chat {
    display: block;
    flex: 1;
    min-height: 0;
    min-width: 0;
  }
  .status {
    margin: 1rem;
    font-size: 0.85rem;
    color: var(--text-muted);
  }
  .error {
    color: var(--danger-text);
  }
</style>
