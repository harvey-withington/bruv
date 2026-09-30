<script lang="ts">
  // One slide field in the Slide Editor: a literal input (text/longtext/media)
  // OR — when a card is linked — bound to a compatible card block, shown as a
  // live-resolved value. Owns its own picker-open state so the editor doesn't
  // have to track it per field. Extracted from SlideEditorDialog to keep that
  // component under the size guideline.
  import type { SlideFieldDef, Block } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { clickOutside } from '../lib/actions'
  import { pushKeyLayer } from '../lib/keyLayer'
  import { Link2, X } from 'lucide-svelte'
  import MediaRefList from './MediaRefList.svelte'

  type AttachOption = { ref: string; name: string; fromLinked: boolean }

  let {
    field,
    value,
    isLinked,
    binding,
    boundLabel,
    boundPreview,
    compatibleBlocks,
    attachmentOptions,
    refDisplayName,
    onInput,
    onBind,
    onUnbind,
  }: {
    field: SlideFieldDef
    value: string
    isLinked: boolean
    binding: string | undefined
    boundLabel: string
    boundPreview: string
    compatibleBlocks: Block[]
    attachmentOptions: AttachOption[]
    refDisplayName: (ref: string) => string
    onInput: (value: string) => void
    onBind: (blockId: string) => void
    onUnbind: () => void
  } = $props()

  let pickerOpen = $state(false)
  const isMedia = $derived(field.type === 'image' || field.type === 'video')
  const label = $derived(t('slide.field.' + field.key))

  function closeDropdowns(): void {
    pickerOpen = false
  }

  // Escape closes an open dropdown. The dropdown pushes a key layer while
  // open — above SlideEditorDialog's — so it owns the Esc and the dialog
  // stays open (topmost-owns-Escape layering, UI-CONVENTIONS §8.1).
  $effect(() => {
    if (!pickerOpen) return
    const layer = pushKeyLayer({ onEscape: closeDropdowns })
    return () => layer.remove()
  })
</script>

<div class="field" use:clickOutside={{ onOutsideClick: closeDropdowns }}>
  <div class="field-head">
    <span class="field-label">{label}</span>
    {#if isLinked}
      {#if binding}
        <button class="link-btn active" type="button" onclick={onUnbind} title={t('slide.unbind')}>
          <Link2 size={11} /> {boundLabel} <X size={10} />
        </button>
      {:else}
        <button class="link-btn" type="button" onclick={() => (pickerOpen = !pickerOpen)} title={t('slide.bind')} aria-label={t('slide.bind')}>
          <Link2 size={11} />
        </button>
      {/if}
    {/if}
  </div>

  {#if binding}
    <div class="bound-value">{boundPreview}</div>
  {:else}
    {#if pickerOpen}
      <div class="block-picker">
        {#each compatibleBlocks as b (b.id)}
          <button type="button" onclick={() => { pickerOpen = false; onBind(b.id) }}>{b.label || b.type}</button>
        {/each}
        {#if compatibleBlocks.length === 0}
          <span class="picker-empty">{t('slide.no_compatible_blocks')}</span>
        {/if}
      </div>
    {/if}
    {#if field.type === 'longtext'}
      <textarea class="field-input" rows="3" {value} oninput={(e) => onInput(e.currentTarget.value)} placeholder={label}></textarea>
    {:else if isMedia}
      <MediaRefList {value} multiple={field.type === 'image'} {attachmentOptions} {refDisplayName} onChange={onInput} />
    {:else}
      <input class="field-input" {value} oninput={(e) => onInput(e.currentTarget.value)} placeholder={label} />
    {/if}
  {/if}
</div>

<style>
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .field-label {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }
  .field-input {
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text-primary);
    font-size: 13px;
    width: 100%;
    box-sizing: border-box;
    font-family: inherit;
  }
  .field-input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .link-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 2px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text-muted);
    font-size: 10px;
    cursor: pointer;
    max-width: 60%;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .link-btn:hover {
    color: var(--text-primary);
    border-color: var(--border-muted);
  }
  .link-btn.active {
    color: var(--accent);
    border-color: var(--accent);
  }
  .bound-value {
    padding: 6px 10px;
    border: 1px dashed var(--accent);
    border-radius: var(--radius);
    background: color-mix(in srgb, var(--accent) 8%, transparent);
    color: var(--text-primary);
    font-size: 13px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 5rem;
    overflow-y: auto;
  }
  .block-picker {
    display: flex;
    flex-direction: column;
    gap: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 4px;
    background: var(--bg-elevated);
  }
  .block-picker button {
    text-align: left;
    background: none;
    border: none;
    color: var(--text-primary);
    font-size: 12px;
    padding: 4px 6px;
    border-radius: 4px;
    cursor: pointer;
  }
  .block-picker button:hover {
    background: var(--bg-hover);
  }
  .picker-empty {
    font-size: 11px;
    color: var(--text-muted);
    padding: 4px 6px;
  }
</style>
