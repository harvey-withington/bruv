<script lang="ts">
  // A router's "Try it" box: type a message, see how it scores and which
  // model the router would pick — against the settings being edited, with
  // no model called.
  import { t } from '../lib/i18n.svelte'
  import { PreviewLLMRoute } from '@shared/api'
  import type { LLMProviderSummary, LLMRouting, LLMTask, RoutePreview } from '@shared/types'
  import { LLM_TASKS, modelDisplayLabel } from '@shared/modelRefs'

  let { routerId, routing, accounts }: {
    routerId: string
    routing: LLMRouting
    accounts: LLMProviderSummary[]
  } = $props()

  let task = $state<LLMTask>('card_chat')
  let message = $state('')
  let tools = $state(true)
  let preview = $state<RoutePreview | null>(null)
  let error = $state('')

  // Re-run as the user types or edits the router; a short debounce keeps
  // it to one RPC per pause.
  $effect(() => {
    const snapshot = { routing: $state.snapshot(routing), ids: accounts.map(a => a.id), task, message, tools }
    if (!snapshot.message.trim()) {
      preview = null
      return
    }
    const timer = setTimeout(async () => {
      try {
        preview = await PreviewLLMRoute(snapshot.routing, snapshot.ids, routerId, snapshot.task, snapshot.message, snapshot.tools)
        error = ''
      } catch (e: unknown) {
        error = e instanceof Error ? e.message : String(e)
      }
    }, 250)
    return () => clearTimeout(timer)
  })

  let picked = $derived(preview ? routing.models.find(m => m.id === preview!.model_id) : undefined)
  let via = $derived.by(() => {
    if (!preview) return ''
    if (preview.via === 'rule') return t('llm_routing.via_rule', { rule: preview.rule_name || `#${preview.rule_index}` })
    return preview.via ? t(`llm_routing.via_${preview.via}`) : t('llm_routing.try_no_models')
  })
</script>

<div class="try">
  <div class="try-controls">
    <select bind:value={task} aria-label={t('llm_routing.rule_tasks')}>
      {#each LLM_TASKS as x (x)}<option value={x}>{t(`llm_routing.task_${x}`)}</option>{/each}
    </select>
    <label class="try-tools"><input type="checkbox" bind:checked={tools} /> {t('llm_routing.try_tools')}</label>
  </div>
  <textarea rows="2" bind:value={message} placeholder={t('llm_routing.try_placeholder')}></textarea>
  {#if error}
    <p class="try-error">{error}</p>
  {:else if preview}
    <div class="try-result">
      <div class="try-pick">
        <strong>{picked ? modelDisplayLabel(picked) : '—'}</strong>
        <span>{via}</span>
      </div>
      <div class="try-score">
        {t('llm_routing.complexity_line', { band: t(`llm_routing.band_${preview.assessment.band}`), score: preview.assessment.score })}
      </div>
      <ul class="try-signals">
        {#each preview.assessment.signals as s (s.key)}
          <li><span class="pts" class:neg={s.points < 0}>{s.points > 0 ? '+' : ''}{s.points}</span> {t(`llm_routing.signal_${s.key}`)}{s.detail ? ` (${s.detail})` : ''}</li>
        {/each}
      </ul>
    </div>
  {/if}
</div>

<style>
  .try { display: flex; flex-direction: column; gap: 0.35rem; }
  .try-controls { display: flex; align-items: center; gap: 0.6rem; }
  .try-tools { display: flex; align-items: center; gap: 0.25rem; font-size: 0.75rem; color: var(--text-secondary); }
  textarea, select {
    padding: 0.3rem 0.45rem; border-radius: 5px; border: 1px solid var(--border);
    background: var(--bg-base); color: var(--text-primary);
    font-size: 0.8rem; font-family: inherit; outline: none; resize: vertical;
  }
  textarea:focus, select:focus { border-color: var(--accent); }
  .try-error { margin: 0; font-size: 0.75rem; color: var(--danger); }
  .try-result {
    padding: 0.45rem 0.55rem; border-radius: 6px;
    background: color-mix(in srgb, var(--accent) 7%, transparent);
    font-size: 0.78rem; color: var(--text-body);
  }
  .try-pick { display: flex; gap: 0.5rem; align-items: baseline; flex-wrap: wrap; }
  .try-pick span, .try-score { color: var(--text-secondary); }
  .try-signals { margin: 0.25rem 0 0; padding-left: 0; list-style: none; font-size: 0.72rem; color: var(--text-muted); }
  .pts { display: inline-block; min-width: 2.2em; font-variant-numeric: tabular-nums; color: var(--success); }
  .pts.neg { color: var(--danger); }
</style>
