<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asNumber, withValue } from './narrow'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const max = $derived(Math.max(1, block.meta?.max ?? 5))
  const current = $derived(Math.max(0, Math.min(asNumber(block.value) ?? 0, max)))
  const percent = $derived(Math.round((current / max) * 100))

  function setStep(value: number) {
    onChange(withValue(block, current === value ? value - 1 : value))
  }
</script>

<div class="progress" role="group" aria-label={t('block.progress.steps', { value: current, max })}>
  <div class="bar" aria-hidden="true">
    <div class="fill" style:width={`${percent}%`}></div>
  </div>
  <div class="steps">
    {#each Array(max) as _, i}
      {@const v = i + 1}
      <button
        type="button"
        class="step"
        class:done={v <= current}
        onclick={() => setStep(v)}
        aria-label={`${v}`}
        aria-pressed={v <= current}
      >
        {v}
      </button>
    {/each}
  </div>
  <span class="count">{t('block.progress.steps', { value: current, max })}</span>
</div>

<style>
  .progress {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .bar {
    height: 8px;
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 4px;
    overflow: hidden;
  }
  .fill {
    height: 100%;
    background: var(--accent);
    transition: width 200ms ease;
  }
  .steps {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }
  .step {
    min-width: 36px;
    min-height: 36px;
    border: 1px solid var(--border);
    background: var(--bg-elev-1);
    color: var(--text-muted);
    border-radius: 6px;
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
    padding: 0.3rem 0.55rem;
  }
  .step:hover,
  .step:focus-visible {
    border-color: var(--accent);
    color: var(--text);
    outline: none;
  }
  .step.done {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .count {
    font-size: 0.8rem;
    color: var(--text-muted);
  }
</style>
