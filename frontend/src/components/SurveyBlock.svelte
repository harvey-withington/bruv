<script lang="ts">
  import { Trash2, Plus } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import type { SurveyQuestion, SurveyQuestionType } from '../lib/types'

  let {
    value = [],
    onUpdate,
  }: {
    value?: SurveyQuestion[]
    onUpdate?: (questions: SurveyQuestion[]) => void
  } = $props()

  // Normalise: old or missing value → empty array
  const questions = $derived(Array.isArray(value) ? value : [])

  function emit(next: SurveyQuestion[]) {
    onUpdate?.(next)
  }

  function newId(): string {
    return `sq-${crypto.randomUUID().slice(0, 8)}`
  }

  function addQuestion() {
    const q: SurveyQuestion = {
      id: newId(),
      prompt: '',
      type: 'text',
      answer: '',
    }
    emit([...questions, q])
  }

  function updateQuestion(id: string, patch: Partial<SurveyQuestion>) {
    emit(questions.map(q => q.id === id ? { ...q, ...patch } : q))
  }

  function removeQuestion(id: string) {
    emit(questions.filter(q => q.id !== id))
  }

  function changeType(id: string, type: SurveyQuestionType) {
    const q = questions.find(x => x.id === id)
    if (!q) return
    let patch: Partial<SurveyQuestion> = { type }
    if (type === 'choice') {
      patch = { type, options: q.options?.length ? q.options : ['Option 1', 'Option 2'], answer: q.multi ? [] : '' }
    } else if (type === 'rating') {
      patch = { type, max: q.max ?? 5, answer: 0 }
    } else {
      patch = { type, answer: '' }
    }
    updateQuestion(id, patch)
  }

  function setChoice(id: string, option: string, checked: boolean) {
    const q = questions.find(x => x.id === id)
    if (!q) return
    if (q.multi) {
      const current = Array.isArray(q.answer) ? q.answer : []
      const next = checked ? [...current, option] : current.filter(x => x !== option)
      updateQuestion(id, { answer: next })
    } else {
      updateQuestion(id, { answer: option })
    }
  }

  function addOption(id: string) {
    const q = questions.find(x => x.id === id)
    if (!q) return
    const next = [...(q.options ?? []), `Option ${(q.options?.length ?? 0) + 1}`]
    updateQuestion(id, { options: next })
  }

  function updateOption(id: string, index: number, text: string) {
    const q = questions.find(x => x.id === id)
    if (!q || !q.options) return
    const options = [...q.options]
    options[index] = text
    updateQuestion(id, { options })
  }

  function removeOption(id: string, index: number) {
    const q = questions.find(x => x.id === id)
    if (!q || !q.options) return
    const options = q.options.filter((_, i) => i !== index)
    updateQuestion(id, { options })
  }

  function ratingStars(max: number): number[] {
    return Array.from({ length: max }, (_, i) => i + 1)
  }
</script>

<div class="survey-block">
  {#if questions.length === 0}
    <p class="survey-empty">{t('block.survey.empty')}</p>
  {/if}

  {#each questions as q (q.id)}
    <div class="survey-question action-reveal-parent">
      <div class="survey-question-head">
        <input
          class="survey-prompt"
          type="text"
          bind:value={q.prompt}
          onchange={(e) => updateQuestion(q.id, { prompt: (e.target as HTMLInputElement).value })}
          placeholder={t('block.survey.question_placeholder')}
        />
        <select
          class="survey-type-select"
          value={q.type}
          onchange={(e) => changeType(q.id, (e.target as HTMLSelectElement).value as SurveyQuestionType)}
        >
          <option value="text">{t('block.survey.type_text')}</option>
          <option value="rating">{t('block.survey.type_rating')}</option>
          <option value="choice">{t('block.survey.type_choice')}</option>
        </select>
        <button
          class="survey-remove action-reveal action-reveal--danger"
          title={t('block.survey.remove_question')}
          onclick={() => removeQuestion(q.id)}
        ><Trash2 size={12} /></button>
      </div>

      {#if q.type === 'text'}
        <textarea
          class="survey-answer-text"
          value={typeof q.answer === 'string' ? q.answer : ''}
          onchange={(e) => updateQuestion(q.id, { answer: (e.target as HTMLTextAreaElement).value })}
          placeholder={t('block.survey.answer_placeholder')}
          rows="2"
        ></textarea>
      {:else if q.type === 'rating'}
        <div class="survey-rating">
          {#each ratingStars(q.max ?? 5) as n}
            <button
              class="survey-star"
              class:filled={typeof q.answer === 'number' && q.answer >= n}
              onclick={() => updateQuestion(q.id, { answer: (q.answer === n ? 0 : n) })}
              aria-label={`${n}`}
            >★</button>
          {/each}
        </div>
      {:else if q.type === 'choice'}
        <div class="survey-choices">
          <label class="survey-multi-toggle">
            <input
              type="checkbox"
              checked={q.multi ?? false}
              onchange={(e) => updateQuestion(q.id, {
                multi: (e.target as HTMLInputElement).checked,
                answer: (e.target as HTMLInputElement).checked ? [] : '',
              })}
            />
            <span>{t('block.survey.multi_select')}</span>
          </label>
          {#each (q.options ?? []) as opt, i}
            <div class="survey-option-row action-reveal-parent">
              <input
                type={q.multi ? 'checkbox' : 'radio'}
                name={`survey-${q.id}`}
                checked={q.multi
                  ? Array.isArray(q.answer) && q.answer.includes(opt)
                  : q.answer === opt}
                onchange={(e) => setChoice(q.id, opt, (e.target as HTMLInputElement).checked)}
              />
              <input
                class="survey-option-text"
                type="text"
                value={opt}
                onchange={(e) => updateOption(q.id, i, (e.target as HTMLInputElement).value)}
                placeholder={t('block.survey.option_placeholder')}
              />
              <button
                class="survey-option-remove action-reveal action-reveal--danger"
                onclick={() => removeOption(q.id, i)}
                title={t('block.survey.remove_question')}
              ><Trash2 size={10} /></button>
            </div>
          {/each}
          <button class="survey-add-option" onclick={() => addOption(q.id)}>
            <Plus size={12} /> {t('block.survey.add_option')}
          </button>
        </div>
      {/if}
    </div>
  {/each}

  <button class="survey-add-question" onclick={addQuestion}>
    <Plus size={14} /> {t('block.survey.add_question')}
  </button>
</div>

<style>
  .survey-block {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .survey-empty {
    font-size: 0.8rem;
    color: var(--text-muted);
    font-style: italic;
    margin: 0;
    padding: 0.25rem 0;
  }

  .survey-question {
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.5rem;
    background: var(--bg-elevated);
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .survey-question-head {
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }

  .survey-prompt {
    flex: 1;
    padding: 0.3rem 0.5rem;
    border: 1px solid transparent;
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 0.9rem;
    font-family: inherit;
    font-weight: 500;
  }
  .survey-prompt:focus {
    border-color: var(--accent);
    outline: none;
  }

  .survey-type-select {
    padding: 0.25rem 0.4rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-body);
    font-size: 0.8rem;
    font-family: inherit;
    cursor: pointer;
  }

  .survey-remove,
  .survey-option-remove {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.2rem;
    line-height: 1;
    display: flex;
    align-items: center;
    border-radius: 3px;
  }
  .survey-remove:hover,
  .survey-option-remove:hover {
    color: var(--danger, #e53935);
    background: var(--bg-surface);
  }

  .survey-answer-text {
    width: 100%;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    resize: vertical;
  }
  .survey-answer-text:focus { border-color: var(--accent); outline: none; }

  .survey-rating {
    display: flex;
    gap: 0.2rem;
  }
  .survey-star {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 1.2rem;
    color: var(--text-faint);
    padding: 0 0.1rem;
    line-height: 1;
  }
  .survey-star.filled,
  .survey-star:hover {
    color: var(--accent-warn, #f2b01e);
  }

  .survey-choices {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .survey-multi-toggle {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    cursor: pointer;
  }

  .survey-option-row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .survey-option-text {
    flex: 1;
    padding: 0.25rem 0.4rem;
    border: 1px solid transparent;
    border-radius: 3px;
    background: transparent;
    color: var(--text-body);
    font-size: 0.85rem;
    font-family: inherit;
  }
  .survey-option-text:focus {
    border-color: var(--border);
    background: var(--bg-surface);
    outline: none;
  }

  .survey-add-option,
  .survey-add-question {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: none;
    border: 1px dashed var(--border);
    border-radius: 4px;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 0.35rem 0.6rem;
    cursor: pointer;
    font-family: inherit;
    align-self: flex-start;
  }
  .survey-add-option:hover,
  .survey-add-question:hover {
    color: var(--accent);
    border-color: var(--accent);
    background: var(--bg-surface);
  }
</style>
