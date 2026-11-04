<script lang="ts">
  // A card's chat on the UNCLI engine (opt-in, Settings → AI): UNCLI's
  // <uncli-chat> in the card's chat panel, showing the card's session
  // (shared/uncliChat.openCardSession). The element shows UNCLI's own
  // download and sign-in steps until its CLI is ready, and approvals
  // inline. BRUV supplies its sign-in, @mentions of cards, bruv: links
  // and the open card as each turn's context (cardChatHost).
  import { untrack } from 'svelte'
  import { BotMessageSquare } from 'lucide-svelte'
  import { GetCardUncliSession, SetCardUncliSession, SearchCards } from '@shared/api'
  import { httpBase, resolveTransportInfo } from '@shared/adapters/cloud'
  import { errorDetail } from '@shared/errors'
  import { cardChatHost, openCardSession, uncliSrc, UNCLI_LINK_SCHEMES, type UncliChatElement } from '@shared/uncliChat'
  import type { Card } from '@shared/types'
  import ChatPanelShell from './ChatPanelShell.svelte'
  import { followBruv } from '../lib/bruvLinks'
  import { theme } from '../lib/theme.svelte'
  import { t } from '../lib/i18n.svelte'

  let {
    cardId,
    card,
    visible = $bindable(false),
    mainWidth = $bindable(720),
    splitterDragging = $bindable(false),
  }: {
    cardId: string
    card: Card | null
    visible: boolean
    mainWidth?: number
    splitterDragging?: boolean
  } = $props()

  let src = $state('')
  let token = ''
  let error = $state('')
  let node = $state<HTMLElement | null>(null)

  // The element (about 200 KB) and the server's address load when a
  // chat first opens, not with the app.
  async function prepare() {
    try {
      const [info] = await Promise.all([resolveTransportInfo(), import('@uncli/chat/element')])
      token = info.token
      src = uncliSrc(httpBase(info))
    } catch (e) {
      error = t('uncli.load_failed', { detail: errorDetail(e) })
    }
  }
  $effect(() => {
    if (visible && !src && !error) untrack(() => void prepare())
  })

  // The host before src: the element connects as soon as it has a src,
  // and its first requests must already carry BRUV's sign-in. Then the
  // card's session.
  $effect(() => {
    const el = node as UncliChatElement | null
    const id = cardId
    if (!el || !src) return
    untrack(() => {
      el.host = cardChatHost({
        token: () => token,
        follow: followBruv,
        searchCards: q => SearchCards(q, 8),
        card: () => card,
        cardLabel: t('uncli.mention_card'),
      })
      el.setAttribute('src', src)
      openCardSession(el, id, { get: GetCardUncliSession, set: SetCardUncliSession }).catch(e => {
        error = t('uncli.session_failed', { detail: errorDetail(e) })
      })
    })
  })
</script>

<ChatPanelShell {visible} bind:mainWidth bind:splitterDragging>
  <div class="uncli-header">
    <span class="uncli-title"><BotMessageSquare size={15} /> {t('chat.title')}</span>
    <span class="uncli-engine" title={t('uncli.engine_hint')}>{t('uncli.engine_badge')}</span>
  </div>
  {#if error}
    <p class="uncli-error" role="alert">{error}</p>
  {:else if src}
    <!-- src is set by the effect above, after the host. -->
    <uncli-chat bind:this={node} scheme={theme.mode} link-schemes={UNCLI_LINK_SCHEMES} storage-key="bruv-card-chat"></uncli-chat>
  {:else}
    <p class="uncli-loading">{t('chat.loading')}</p>
  {/if}
</ChatPanelShell>

<style>
  /* Matches ChatSection's header: icon + Proper Case title (UI-CONVENTIONS
     §13), clear of CardDetail's close button at the top right. */
  .uncli-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.65rem 2.75rem 0.65rem 1rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }
  .uncli-title {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.82rem;
    font-weight: 600;
    color: var(--text-strong);
  }
  .uncli-engine {
    margin-left: auto;
    font-size: 0.7rem;
    color: var(--text-muted);
    border: 1px solid var(--border-muted);
    border-radius: 999px;
    padding: 0.05rem 0.5rem;
  }
  uncli-chat {
    display: block;
    flex: 1;
    min-height: 0;
    min-width: 0;
  }
  .uncli-error,
  .uncli-loading {
    margin: 1rem;
    font-size: 0.8rem;
    color: var(--text-muted);
  }
  .uncli-error {
    color: var(--danger-light);
  }
</style>
