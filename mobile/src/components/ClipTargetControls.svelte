<script lang="ts">
  import { Layers, MapPin, X } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { DECK_PIN_SENTINEL, type CapturePrefs } from '../lib/capturePrefs'
  import DeckTargetPicker from './DeckTargetPicker.svelte'
  import ClipPinPicker from './ClipPinPicker.svelte'

  // The "where does this land?" controls shared by both share modes:
  // an include-in-deck toggle plus the deck target, and (clip mode only)
  // the pin destination. Every choice is sticky — the host persists the
  // prefs it gets back through onChange.

  let {
    prefs,
    onChange,
    disabled = false,
    showPin = false,
  }: {
    prefs: CapturePrefs
    onChange: (next: CapturePrefs) => void
    disabled?: boolean
    showPin?: boolean
  } = $props()

  let deckPickerOpen = $state(false)
  let pinPickerOpen = $state(false)

  const deckName = $derived(prefs.deckTarget?.name ?? '')

  const pinLabel = $derived(
    prefs.categoryID === ''
      ? t('clip.pin_inbox')
      : prefs.categoryID === DECK_PIN_SENTINEL
        ? t('clip.pin_with_deck')
        : prefs.categoryName || t('clip.pin_category_unnamed'),
  )

  function toggleDeck(on: boolean) {
    onChange({ ...prefs, includeInDeck: on })
    // Turning it on without a target is a dead end — go straight to the
    // picker rather than making the user find the row below.
    if (on && !prefs.deckTarget) deckPickerOpen = true
  }

  function chooseDeck(target: CapturePrefs['deckTarget']) {
    deckPickerOpen = false
    // Picking a deck is itself the intent to use it.
    onChange({ ...prefs, deckTarget: target, includeInDeck: target ? true : prefs.includeInDeck })
  }

  function clearDeck() {
    // The pin choice can't outlive the deck it referred to.
    onChange({
      ...prefs,
      deckTarget: null,
      includeInDeck: false,
      categoryID: prefs.categoryID === DECK_PIN_SENTINEL ? '' : prefs.categoryID,
      categoryName: prefs.categoryID === DECK_PIN_SENTINEL ? '' : prefs.categoryName,
    })
  }

  function choosePin(sel: { categoryID: string; categoryName: string }) {
    pinPickerOpen = false
    onChange({ ...prefs, categoryID: sel.categoryID, categoryName: sel.categoryName })
  }
</script>

<div class="controls">
  <label class="toggle-row">
    <input
      type="checkbox"
      checked={prefs.includeInDeck}
      onchange={(e) => toggleDeck((e.currentTarget as HTMLInputElement).checked)}
      {disabled}
    />
    <span class="toggle-label">{t('clip.include_in_deck')}</span>
  </label>

  <div class="target-row">
    <button type="button" class="target" onclick={() => (deckPickerOpen = true)} {disabled}>
      <Layers size={14} />
      <span class="target-label">{t('clip.deck_target')}</span>
      <span class="target-value" class:muted={!deckName}>{deckName || t('clip.none')}</span>
    </button>
    {#if prefs.deckTarget}
      <button
        type="button"
        class="clear"
        onclick={clearDeck}
        {disabled}
        aria-label={t('clip.clear_deck')}
        title={t('clip.clear_deck')}
      >
        <X size={14} />
      </button>
    {/if}
  </div>

  {#if showPin}
    <div class="target-row">
      <button type="button" class="target" onclick={() => (pinPickerOpen = true)} {disabled}>
        <MapPin size={14} />
        <span class="target-label">{t('clip.pin')}</span>
        <span class="target-value">{pinLabel}</span>
      </button>
    </div>
  {/if}
</div>

{#if deckPickerOpen}
  <DeckTargetPicker onSelect={chooseDeck} onClose={() => (deckPickerOpen = false)} />
{/if}

{#if pinPickerOpen}
  <ClipPinPicker
    current={prefs.categoryID}
    hasDeck={!!prefs.deckTarget}
    onSelect={choosePin}
    onClose={() => (pinPickerOpen = false)}
  />
{/if}

<style>
  .controls {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.75rem 0.85rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 10px;
  }

  .toggle-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    cursor: pointer;
    min-height: 32px;
  }

  .toggle-row input {
    width: 20px;
    height: 20px;
    accent-color: var(--accent);
    flex-shrink: 0;
  }

  .toggle-label {
    font-size: 0.9rem;
    color: var(--text);
  }

  .target-row {
    display: flex;
    align-items: stretch;
    gap: 0.35rem;
  }

  .target {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.55rem 0.7rem;
    color: var(--text-muted);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
    text-align: left;
    touch-action: manipulation;
  }
  .target:hover:not(:disabled),
  .target:focus-visible:not(:disabled) {
    border-color: var(--accent);
    outline: none;
  }

  .target-label {
    text-transform: uppercase;
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    flex-shrink: 0;
  }

  .target-value {
    flex: 1;
    min-width: 0;
    text-align: right;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .target-value.muted {
    color: var(--text-faint);
  }

  .clear {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0 0.6rem;
    display: inline-flex;
    align-items: center;
    touch-action: manipulation;
  }
  .clear:hover:not(:disabled),
  .clear:focus-visible:not(:disabled) {
    color: #ef4444;
    border-color: rgba(239, 68, 68, 0.35);
    outline: none;
  }

  .target:disabled,
  .clear:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
