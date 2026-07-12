<script lang="ts">
  import { Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { navigate, cardURL } from '../lib/router.svelte'
  import QuickCaptureSheet from './QuickCaptureSheet.svelte'

  // Topbar icon button that opens the quick capture sheet. Sits next
  // to the Search icon on Browse / Inbox / Project topbars (replaces
  // the old floating action button, which overlaid page content).

  let open = $state(false)

  function handleSaved(cardID: string) {
    // After a quick capture, drop the user into the new card so they
    // can elaborate or move on — same intent as desktop's Inbox auto-
    // open after card creation. They can hit Back to return to where
    // they were capturing from.
    navigate(cardURL(cardID))
  }
</script>

<button
  type="button"
  class="icon-btn"
  onclick={() => (open = true)}
  aria-label={t('capture.button_label')}
  title={t('capture.button_label')}
>
  <Plus size={18} />
</button>

{#if open}
  <QuickCaptureSheet onClose={() => (open = false)} onSaved={handleSaved} />
{/if}

<style>
  /* Matches the topbar icon-button pattern (Search / Bell / Settings). */
  .icon-btn {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
    min-width: 40px;
    min-height: 40px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  .icon-btn:hover,
  .icon-btn:focus-visible {
    color: var(--text);
    background: var(--bg-elev-1);
    outline: none;
  }
</style>
