<script lang="ts">
  // One slide field in the Slide Editor: a literal input (text/longtext/media)
  // OR — when a card is linked — bound to a compatible card block, shown as a
  // live-resolved value. Owns its own picker-open state so the editor doesn't
  // have to track it per field. Extracted from SlideEditorDialog to keep that
  // component under the size guideline.
  import type { SlideFieldDef, Block } from '@shared/types'
  import { t } from '../lib/i18n.svelte'
  import { clickOutside } from '../lib/actions'
  import { Link2, Paperclip, X } from 'lucide-svelte'

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
    onPickAttachment,
    onClearAttachment,
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
    onPickAttachment: (ref: string) => void
    onClearAttachment: () => void
  } = $props()

  let pickerOpen = $state(false)
  let attachOpen = $state(false)
  const isMedia = $derived(field.type === 'image' || field.type === 'video')
  const isAttachmentRef = $derived(value.startsWith('attachment:'))
  const label = $derived(t('slide.field.' + field.key))

  function closeDropdowns(): void {
    pickerOpen = false
    attachOpen = false
  }

  // Escape closes an open dropdown. Registered CAPTURE-phase only while a
  // dropdown is open, so it runs after SlideEditorDialog's capture handler —
  // which sees the open .block-picker and leaves the dialog open, deferring
  // the actual close to this listener (topmost-owns-Escape layering).
  $effect(() => {
    if (!pickerOpen && !attachOpen) return
    const onKeydown = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.preventDefault()
      closeDropdowns()
    }
    window.addEventListener('keydown', onKeydown, true)
    return () => window.removeEventListener('keydown', onKeydown, true)
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
      {#if isAttachmentRef}
        <div class="attach-chip">
          <Paperclip size={12} />
          <span class="attach-name">{refDisplayName(value)}</span>
          <button class="chip-x" type="button" onclick={onClearAttachment} title={t('common.delete')} aria-label={t('common.delete')}><X size={12} /></button>
        </div>
      {:else}
        <input class="field-input mono" {value} oninput={(e) => onInput(e.currentTarget.value)} placeholder={t('slide.media_placeholder')} />
        {#if attachmentOptions.length > 0}
          <button class="attach-toggle" type="button" onclick={() => (attachOpen = !attachOpen)}>
            <Paperclip size={11} /> {t('slide.pick_attachment')}
          </button>
          {#if attachOpen}
            <div class="block-picker">
              {#each attachmentOptions as opt (opt.ref)}
                <button type="button" onclick={() => { attachOpen = false; onPickAttachment(opt.ref) }}>
                  {opt.name}{opt.fromLinked ? ` ${t('slide.from_linked_card')}` : ''}
                </button>
              {/each}
            </div>
          {/if}
        {:else}
          <span class="field-hint">{t('slide.media_hint')}</span>
        {/if}
      {/if}
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
  .field-input.mono {
    font-family: ui-monospace, Consolas, monospace;
    font-size: 12px;
  }
  .field-hint {
    font-size: 10px;
    color: var(--text-muted);
    font-style: italic;
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
  .attach-chip {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
  }
  .attach-name {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .attach-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    align-self: flex-start;
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 11px;
    cursor: pointer;
    padding: 2px 0;
  }
  .attach-toggle:hover {
    color: var(--text-primary);
  }
  .chip-x {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    display: flex;
    padding: 0;
  }
  .chip-x:hover {
    color: var(--danger);
  }
</style>
