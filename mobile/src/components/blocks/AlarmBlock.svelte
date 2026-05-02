<script lang="ts">
  import { Bell, BellRing } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { withMeta } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const fired = $derived(!!block.meta?.alarm_fired)
  const channels = $derived(block.meta?.alarm_channels ?? 'in-app')

  // ISO datetime → local datetime-local input value.
  const inputVal = $derived(toInputValue(block.meta?.alarm_time ?? ''))

  function toInputValue(raw: string): string {
    if (!raw) return ''
    const d = new Date(raw)
    if (Number.isNaN(d.getTime())) return ''
    const yr = d.getFullYear().toString().padStart(4, '0')
    const mo = (d.getMonth() + 1).toString().padStart(2, '0')
    const dy = d.getDate().toString().padStart(2, '0')
    const hr = d.getHours().toString().padStart(2, '0')
    const mn = d.getMinutes().toString().padStart(2, '0')
    return `${yr}-${mo}-${dy}T${hr}:${mn}`
  }

  function fromInputValue(s: string): string {
    if (!s) return ''
    const d = new Date(s)
    if (Number.isNaN(d.getTime())) return s
    return d.toISOString()
  }

  function setTime(e: Event) {
    const next = (e.currentTarget as HTMLInputElement).value
    onChange(withMeta(block, { alarm_time: fromInputValue(next), alarm_fired: false }))
  }

  function toggleChannel(ch: string) {
    const list = channels.split(',').map((s) => s.trim()).filter(Boolean)
    const idx = list.indexOf(ch)
    if (idx === -1) list.push(ch)
    else list.splice(idx, 1)
    onChange(withMeta(block, { alarm_channels: list.join(',') }))
  }

  function isOn(ch: string): boolean {
    return channels.split(',').map((s) => s.trim()).includes(ch)
  }
</script>

<div class="alarm">
  <header class="alarm-header">
    {#if fired}
      <BellRing size={16} class="bell-fired" />
      <span class="status fired">{t('block.alarm.fired')}</span>
    {:else}
      <Bell size={16} class="bell-pending" />
      <span class="status">{t('block.alarm.scheduled')}</span>
    {/if}
  </header>

  <label class="row">
    <span class="key">{t('block.alarm.time')}</span>
    <input type="datetime-local" class="time" value={inputVal} oninput={setTime} />
  </label>

  <div class="row">
    <span class="key">{t('block.alarm.channels')}</span>
    <div class="channels">
      <button type="button" class="ch" class:on={isOn('in-app')} onclick={() => toggleChannel('in-app')}>
        in-app
      </button>
      <button type="button" class="ch" class:on={isOn('system')} onclick={() => toggleChannel('system')}>
        system
      </button>
    </div>
  </div>
</div>

<style>
  .alarm {
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 0.65rem 0.75rem;
  }
  .alarm-header {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  :global(.bell-pending) {
    color: var(--text-muted);
  }
  :global(.bell-fired) {
    color: var(--accent);
  }
  .status {
    font-size: 0.8rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .status.fired {
    color: var(--accent);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
  .key {
    font-size: 0.8rem;
    color: var(--text-muted);
    min-width: 4.5rem;
  }
  .time {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
    padding: 0.4rem 0.55rem;
    color-scheme: dark light;
  }
  .time:focus {
    outline: none;
    border-color: var(--accent);
  }
  .channels {
    display: inline-flex;
    gap: 0.3rem;
  }
  .ch {
    background: var(--bg);
    border: 1px solid var(--border);
    color: var(--text-muted);
    padding: 0.3rem 0.6rem;
    border-radius: 6px;
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .ch:hover,
  .ch:focus-visible {
    color: var(--text);
    border-color: var(--text-muted);
    outline: none;
  }
  .ch.on {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
</style>
