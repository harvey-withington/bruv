<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { Bell, BellOff, BellRing, RotateCcw } from 'lucide-svelte'
  import type { BlockMeta } from '@shared/types'

  let {
    value,
    meta = {},
    onUpdate
  }: {
    value: string | null
    meta: BlockMeta
    onUpdate: (value: string | null, meta?: BlockMeta) => void
  } = $props()

  const alarmTime = $derived(meta?.alarm_time || '')
  const alarmFired = $derived(meta?.alarm_fired || false)
  const alarmChannels = $derived(meta?.alarm_channels || 'in-app,system')

  // svelte-ignore state_referenced_locally
  let channelSystem = $state(alarmChannels.includes('system'))
  // svelte-ignore state_referenced_locally
  let channelEmail = $state(alarmChannels.includes('email'))

  function setAlarm(e: Event) {
    const target = e.target as HTMLInputElement
    const dt = target.value
    if (!dt) return
    const iso = new Date(dt).toISOString()
    const channels = buildChannels()
    onUpdate(dt, { ...meta, alarm_time: iso, alarm_fired: false, alarm_channels: channels })
  }

  function clearAlarm() {
    onUpdate(null, { ...meta, alarm_time: undefined, alarm_fired: false })
  }

  function resetAlarm() {
    onUpdate(value, { ...meta, alarm_fired: false })
  }

  function buildChannels(): string {
    const ch = ['in-app']
    if (channelSystem) ch.push('system')
    if (channelEmail) ch.push('email')
    return ch.join(',')
  }

  function updateChannels() {
    const channels = buildChannels()
    onUpdate(value, { ...meta, alarm_channels: channels })
  }

  function formatAlarm(iso: string): string {
    try {
      return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
    } catch { return iso }
  }
</script>

<div class="alarm-block">
  <div class="alarm-row">
    {#if alarmFired}
      <BellRing size={16} />
      <span class="alarm-status fired">{t('block.alarm_fired')}</span>
      <button class="alarm-reset-btn" onclick={resetAlarm} title={t('block.alarm_reset')}><RotateCcw size={12} /></button>
    {:else if alarmTime}
      <Bell size={16} />
      <span class="alarm-status set">{formatAlarm(alarmTime)}</span>
      <button class="alarm-clear-btn" onclick={clearAlarm}><BellOff size={12} /></button>
    {:else}
      <BellOff size={16} />
      <span class="alarm-status muted">{t('block.alarm_not_set')}</span>
    {/if}
  </div>
  <div class="alarm-input-row">
    <input
      type="datetime-local"
      class="alarm-datetime"
      value={alarmTime ? new Date(alarmTime).toISOString().slice(0, 16) : ''}
      onchange={setAlarm}
    />
  </div>
  <div class="alarm-channels">
    <label class="alarm-channel"><input type="checkbox" checked={channelSystem} onchange={() => { channelSystem = !channelSystem; updateChannels() }} /> {t('agent.channel_system')}</label>
    <label class="alarm-channel"><input type="checkbox" checked={channelEmail} onchange={() => { channelEmail = !channelEmail; updateChannels() }} /> {t('agent.channel_email')}</label>
  </div>
</div>

<style>
  .alarm-block { display: flex; flex-direction: column; gap: 8px; }
  .alarm-row { display: flex; align-items: center; gap: 8px; }
  .alarm-status { font-size: 0.9em; }
  .alarm-status.set { color: var(--accent); }
  .alarm-status.fired { color: var(--danger); }
  .alarm-status.muted { color: var(--text-muted); font-style: italic; }
  .alarm-clear-btn, .alarm-reset-btn {
    background: none; border: none; color: var(--text-muted); cursor: pointer; padding: 2px;
  }
  .alarm-clear-btn:hover { color: var(--danger); }
  .alarm-reset-btn:hover { color: var(--accent); }
  .alarm-input-row { display: flex; gap: 8px; }
  .alarm-datetime {
    padding: 4px 8px; border: 1px solid var(--border); border-radius: 6px;
    background: var(--bg-surface); color: var(--text-primary); font-size: 0.9em;
  }
  .alarm-datetime:focus { border-color: var(--accent); outline: none; }
  .alarm-channels { display: flex; gap: 12px; }
  .alarm-channel { display: flex; align-items: center; gap: 4px; font-size: 0.8em; color: var(--text-muted); cursor: pointer; }
</style>
