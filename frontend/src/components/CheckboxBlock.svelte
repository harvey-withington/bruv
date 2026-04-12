<script lang="ts">
  import { t } from '../lib/i18n.svelte'

  let {
    value,
    onUpdate
  }: {
    value: boolean
    onUpdate: (value: boolean) => void
  } = $props()
</script>

<div class="checkbox-block">
  <label class="toggle-label">
    <input type="checkbox" class="toggle-input" checked={value} onchange={() => onUpdate(!value)} />
    <span class="toggle-switch"></span>
    <span class="toggle-text">{value ? t('block.yes') : t('block.no')}</span>
  </label>
</div>

<style>
  .checkbox-block { display: flex; align-items: center; }
  .toggle-label { display: flex; align-items: center; gap: 8px; cursor: pointer; }
  .toggle-input { display: none; }
  .toggle-switch {
    width: 36px; height: 20px; border-radius: 10px; position: relative;
    background: var(--border); transition: background var(--duration-moderate);
  }
  .toggle-switch::after {
    content: ''; position: absolute; top: 2px; left: 2px;
    width: 16px; height: 16px; border-radius: 50%;
    background: white; transition: transform var(--duration-moderate);
  }
  .toggle-input:checked + .toggle-switch { background: var(--accent); }
  .toggle-input:checked + .toggle-switch::after { transform: translateX(16px); }
  .toggle-text { font-size: 0.9em; color: var(--text-primary); }
</style>
