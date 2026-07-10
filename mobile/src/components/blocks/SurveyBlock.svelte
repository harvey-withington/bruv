<script lang="ts">
  // Survey block — answer-only on mobile. Question authoring (adding
  // / removing / re-typing questions) stays desktop. The user can
  // still answer text / rating / single-choice / multi-choice
  // questions from their phone.

  import { untrack, getContext } from 'svelte'
  import { Star } from 'lucide-svelte'
  import { t } from '../../lib/i18n.svelte'
  import { EDIT_SCOPE_KEY, type EditScope } from '@shared/editScope'
  import type { Block, SurveyQuestion } from '@shared/types'
  import { asSurveyQuestions, withValue } from './narrow'
  import { draftEdit } from '../../lib/actions/draftEdit'
  import EditorDoneButton from '../EditorDoneButton.svelte'

  let { block, onChange }: { block: Block; onChange: (next: Block) => void } = $props()

  const questions = $derived(asSurveyQuestions(block.value))

  const editScope = getContext<EditScope | undefined>(EDIT_SCOPE_KEY) ?? null

  function answer(qID: string, ans: string | string[] | number | undefined) {
    const next = questions.map((q) => (q.id === qID ? { ...q, answer: ans } : q))
    onChange(withValue(block, next))
  }

  // Text answers are draft-based per the keyboard entry contract,
  // mobile multiline variant (draftEdit action: Enter inserts a
  // newline; blur / ✓ Done commit, Escape reverts). Drafts are keyed
  // by stable question ID and exist only while an answer is being
  // edited — outside that window the field renders q.answer, so
  // external edits flow through untouched.
  let textDrafts = $state<Record<string, string>>({})
  // Which question's textarea is focused → shows its ✓ Done button.
  let focusedQuestionID = $state<string | null>(null)
  // Element refs keyed by question ID so ✓ Done can blur the right
  // textarea (blur = the action's commit path).
  const answerEls = new Map<string, HTMLTextAreaElement>()

  function textAnswer(q: SurveyQuestion): string {
    return typeof q.answer === 'string' ? q.answer : ''
  }

  function commitText(q: SurveyQuestion) {
    const draft = textDrafts[q.id]
    if (draft === undefined) return
    delete textDrafts[q.id]
    if (draft !== textAnswer(q)) answer(q.id, draft)
  }

  function revertText(q: SurveyQuestion) {
    delete textDrafts[q.id]
  }

  // Fresh block prop (re-keyed to a different block): drop stale drafts.
  let seededID = untrack(() => block.id)
  $effect(() => {
    if (block.id === seededID) return
    seededID = block.id
    textDrafts = {}
  })

  function isChosen(q: SurveyQuestion, opt: string): boolean {
    if (q.multi && Array.isArray(q.answer)) return q.answer.includes(opt)
    return q.answer === opt
  }

  function toggleChoice(q: SurveyQuestion, opt: string) {
    if (q.multi) {
      const current = Array.isArray(q.answer) ? q.answer : []
      if (current.includes(opt)) {
        answer(q.id, current.filter((x) => x !== opt))
      } else {
        answer(q.id, [...current, opt])
      }
    } else {
      answer(q.id, q.answer === opt ? undefined : opt)
    }
  }
</script>

<div class="survey">
  {#each questions as q (q.id)}
    <div class="question">
      <p class="prompt">{q.prompt}</p>
      {#if q.type === 'text'}
        <textarea
          class="text-answer"
          rows="2"
          value={textDrafts[q.id] ?? textAnswer(q)}
          oninput={(e) => (textDrafts[q.id] = (e.currentTarget as HTMLTextAreaElement).value)}
          onfocus={(e) => { focusedQuestionID = q.id; answerEls.set(q.id, e.currentTarget as HTMLTextAreaElement) }}
          onblur={() => { if (focusedQuestionID === q.id) focusedQuestionID = null }}
          use:draftEdit={{ multiline: true, enterInsertsNewline: true, onCommit: () => commitText(q), onCancel: () => revertText(q), scope: editScope }}
          placeholder={t('block.survey.unanswered')}
        ></textarea>
        {#if focusedQuestionID === q.id}
          <div class="editor-actions">
            <EditorDoneButton onDone={() => answerEls.get(q.id)?.blur()} />
          </div>
        {/if}
      {:else if q.type === 'rating'}
        {@const max = q.max ?? 5}
        {@const current = typeof q.answer === 'number' ? q.answer : 0}
        <div class="stars">
          {#each Array(max) as _, i}
            {@const v = i + 1}
            <button
              type="button"
              class="star"
              class:filled={v <= current}
              onclick={() => answer(q.id, current === v ? 0 : v)}
              aria-label={t('block.rating.set_aria', { value: v })}
            >
              <Star size={22} />
            </button>
          {/each}
        </div>
      {:else if q.type === 'choice'}
        <ul class="choices">
          {#each q.options ?? [] as opt}
            <li>
              <button
                type="button"
                class="choice"
                class:selected={isChosen(q, opt)}
                onclick={() => toggleChoice(q, opt)}
                aria-pressed={isChosen(q, opt)}
              >
                <span class="indicator" class:filled={isChosen(q, opt)} class:multi={q.multi} aria-hidden="true"></span>
                <span class="label">{opt}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/each}
</div>

<style>
  .survey {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }
  .question {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
  }
  .prompt {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 500;
    color: var(--text);
  }
  .text-answer {
    background: var(--bg-elev-1);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    padding: 0.5rem 0.65rem;
    resize: vertical;
  }
  .text-answer:focus {
    outline: none;
    border-color: var(--accent);
  }
  .editor-actions {
    display: flex;
    justify-content: flex-end;
  }
  .stars {
    display: inline-flex;
    gap: 0.2rem;
  }
  .star {
    background: transparent;
    border: none;
    color: var(--text-faint);
    padding: 0.25rem;
    cursor: pointer;
    border-radius: 6px;
    min-width: 36px;
    min-height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .star.filled :global(svg) {
    color: var(--accent);
    fill: var(--accent);
  }
  .star:hover,
  .star:focus-visible {
    background: var(--bg-elev-1);
    outline: none;
  }
  .choices {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .choice {
    display: inline-flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.5rem 0.65rem;
    color: var(--text);
    font: inherit;
    cursor: pointer;
    text-align: left;
    min-height: 44px;
  }
  .choice:hover,
  .choice:focus-visible {
    background: var(--bg-elev-1);
    border-color: var(--border);
    outline: none;
  }
  .choice.selected {
    border-color: var(--accent);
  }
  .indicator {
    width: 16px;
    height: 16px;
    border: 2px solid var(--text-muted);
    background: transparent;
    flex-shrink: 0;
    box-sizing: border-box;
    border-radius: 50%;
  }
  .indicator.multi {
    border-radius: 4px;
  }
  .indicator.filled {
    border-color: var(--accent);
    background: var(--accent);
    box-shadow: inset 0 0 0 3px var(--bg);
  }
  .indicator.multi.filled {
    box-shadow: none;
  }
  .label {
    font-size: 0.95rem;
  }
</style>
