<script lang="ts">
  /**
   * PinPanel — the pin/unpin/move/navigate controls that live in the
   * CardDetail modal subheader. Pure presentational component:
   * renders the buttons, fires callbacks. All state (pinBreadcrumbs,
   * pinActionLoading, etc.) and all mutation logic (UnpinCard,
   * PinCard, GetCardPinBreadcrumbs) lives in the parent — this
   * component just emits intent. Extracted from CardDetail as part
   * of the ≤300-line component refactor.
   */
  import { MapPin, MapPinOff, MoveRight } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import type { CardPin } from '@shared/types'

  let {
    pinBreadcrumbs,
    currentPin,
    isPinnedHere,
    otherPins,
    effectiveCategoryId,
    effectiveCategoryName,
    pinActionLoading,
    showOtherPins = $bindable(),
    onToggleCurrentPin,
    onOpenMovePicker,
    onOpenPinPicker,
    onUnpin,
    onNavigateToPinnedProject,
  }: {
    pinBreadcrumbs: CardPin[]
    currentPin: CardPin | null
    isPinnedHere: boolean
    otherPins: CardPin[]
    effectiveCategoryId: string | null
    effectiveCategoryName: string | null
    pinActionLoading: boolean
    showOtherPins: boolean
    onToggleCurrentPin: () => void
    onOpenMovePicker: (pin: CardPin) => void
    onOpenPinPicker: () => void
    onUnpin: (pin: CardPin) => void
    onNavigateToPinnedProject: (pin: CardPin) => void
  } = $props()
</script>

<div class="modal-subheader">
  {#if effectiveCategoryId}
    <!-- Current-category toggle -->
    <button
      class="pin-toggle"
      class:pinned={isPinnedHere}
      onclick={onToggleCurrentPin}
      disabled={pinActionLoading}
      title={isPinnedHere ? `${t('tooltip.unpin')} "${effectiveCategoryName}"` : `${t('card.pin_to')} "${effectiveCategoryName}"`}
    >
      {#if isPinnedHere}
        <MapPin size={11} />
      {:else}
        <MapPinOff size={11} />
      {/if}
      <span class="pin-toggle-name">{effectiveCategoryName}</span>
    </button>
    {#if currentPin}
      <button class="btn-pin-action" onclick={() => onOpenMovePicker(currentPin)} disabled={pinActionLoading} title={t('tooltip.move_pin')} aria-label={t('tooltip.move_pin')}><MoveRight size={11} /></button>
    {/if}

    <!-- Other pins expandable -->
    <button
      class="btn-other-pins"
      class:expanded={showOtherPins}
      onclick={() => showOtherPins = !showOtherPins}
      disabled={pinActionLoading}
    >
      {t('card.other_pins')} ({otherPins.length}) {showOtherPins ? '▲' : '▼'}
    </button>

  {:else}
    <!-- No context category (inbox) — show pins list directly -->
    {#if pinBreadcrumbs.length === 0}
      <button class="btn-pin" onclick={onOpenPinPicker} disabled={pinActionLoading}><MapPin size={11} /> {t('card.pin_to')}</button>
    {:else}
      {#each pinBreadcrumbs as pin}
        <div class="location-pin">
          <button class="location-breadcrumb" onclick={() => onNavigateToPinnedProject(pin)} title={t('tooltip.go_to_project')}><MapPin size={11} />{pin.breadcrumb}</button>
          <button class="btn-pin-action btn-unpin" onclick={() => onUnpin(pin)} disabled={pinActionLoading} title={t('tooltip.unpin')} aria-label={t('tooltip.unpin')}><MapPinOff size={11} /></button>
        </div>
      {/each}
      <button class="btn-pin" onclick={onOpenPinPicker} disabled={pinActionLoading}>{t('card.pin_to_category')}</button>
    {/if}
  {/if}

  <!-- Expanded other-pins panel — full width below the toggle row -->
  {#if showOtherPins}
    <div class="other-pins-panel">
      {#each otherPins as pin}
        <div class="location-pin">
          <button class="location-breadcrumb" onclick={() => onNavigateToPinnedProject(pin)} title={t('tooltip.go_to_project')}><MapPin size={11} />{pin.breadcrumb}</button>
          <button class="btn-pin-action btn-unpin" onclick={() => onUnpin(pin)} disabled={pinActionLoading} title={t('tooltip.unpin')} aria-label={t('tooltip.unpin')}><MapPinOff size={11} /></button>
        </div>
      {/each}
      <button class="btn-pin" onclick={onOpenPinPicker} disabled={pinActionLoading}>{t('card.pin_to_category')}</button>
    </div>
  {/if}
</div>

<style>
  .modal-subheader {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem 0.5rem;
    padding: 0.35rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    font-size: 0.73rem;
    min-height: 2rem;
    position: relative;
  }

  /* Current-category pin toggle */
  .pin-toggle {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.55rem;
    border-radius: 5px;
    border: 1px solid var(--border-muted);
    background: var(--bg-surface);
    color: var(--text-muted);
    font-size: 0.73rem;
    cursor: pointer;
    transition: background var(--duration-fast), border-color var(--duration-fast), color var(--duration-fast);
  }
  .pin-toggle.pinned {
    border-color: var(--accent);
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .pin-toggle:hover:not(:disabled) {
    border-color: var(--accent);
    color: var(--accent);
  }
  .pin-toggle.pinned:hover:not(:disabled) {
    border-color: #eb5a46;
    color: #eb5a46;
    background: color-mix(in srgb, #eb5a46 8%, transparent);
  }
  .pin-toggle:disabled { opacity: 0.5; cursor: default; }

  .pin-toggle-name {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* "Other pins (N)" button */
  .btn-other-pins {
    font-size: 0.7rem;
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
    border: 1px solid transparent;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    margin-left: auto;
  }
  .btn-other-pins:hover { color: var(--text-body); background: var(--bg-surface); }
  .btn-other-pins.expanded { color: var(--text-body); }
  .btn-other-pins:disabled { opacity: 0.5; cursor: default; }

  /* Expanded other-pins panel — full width below the toggle row */
  .other-pins-panel {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: 0.35rem 0 0.1rem;
    border-top: 1px solid var(--border-muted);
    margin-top: 0.1rem;
  }

  .location-pin {
    display: flex;
    align-items: center;
    gap: 0.2rem;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    padding: 0.1rem 0.2rem 0.1rem 0.45rem;
    max-width: 100%;
  }

  .location-breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--text-body);
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    background: none;
    border: none;
    padding: 0;
    font-size: inherit;
    font-family: inherit;
    cursor: pointer;
    text-align: left;
  }
  .location-breadcrumb:hover {
    color: var(--accent);
    text-decoration: underline;
  }

  .btn-pin {
    font-size: 0.7rem;
    padding: 0.15rem 0.45rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    line-height: 1.4;
    align-self: flex-start;
  }
  .btn-pin:hover { border-color: var(--accent); color: var(--accent); }
  .btn-pin:disabled { opacity: 0.5; cursor: default; }

  .btn-pin-action {
    background: none;
    border: none;
    padding: 0.15rem 0.2rem;
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    line-height: 1;
  }
  .btn-pin-action:hover { color: var(--text-primary); }
  .btn-unpin:hover { color: #eb5a46; }
</style>
