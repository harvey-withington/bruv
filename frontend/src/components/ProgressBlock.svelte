<script lang="ts">
  let {
    value,
    onUpdate
  }: {
    value: number
    onUpdate: (value: number) => void
  } = $props()

  const pct = $derived(Math.max(0, Math.min(100, value || 0)))

  function handleInput(e: Event) {
    const v = (e.target as HTMLInputElement).valueAsNumber
    onUpdate(Math.max(0, Math.min(100, isNaN(v) ? 0 : v)))
  }
</script>

<div class="progress-block">
  <div class="progress-bar-track">
    <div class="progress-bar-fill" style:width="{pct}%"></div>
  </div>
  <div class="progress-controls">
    <input type="range" class="progress-slider" min="0" max="100" value={pct} oninput={handleInput} />
    <span class="progress-label">{pct}%</span>
  </div>
</div>

<style>
  .progress-block { display: flex; flex-direction: column; gap: 6px; }
  .progress-bar-track {
    height: 8px; background: var(--border); border-radius: 4px; overflow: hidden;
  }
  .progress-bar-fill {
    height: 100%; background: var(--accent); border-radius: 4px;
    transition: width var(--duration-moderate) ease;
  }
  .progress-controls { display: flex; align-items: center; gap: 8px; }
  .progress-slider {
    flex: 1; -webkit-appearance: none; appearance: none; height: 4px;
    background: var(--border); border-radius: 2px; outline: none;
  }
  .progress-slider::-webkit-slider-thumb {
    -webkit-appearance: none; width: 14px; height: 14px; border-radius: 50%;
    background: var(--accent); cursor: pointer;
  }
  .progress-slider::-moz-range-thumb {
    width: 14px; height: 14px; border-radius: 50%; border: none;
    background: var(--accent); cursor: pointer;
  }
  .progress-label { font-size: 0.85em; color: var(--text-primary); min-width: 36px; text-align: right; }
</style>
