// Tickable GFM task lists ("- [ ] thing") in rendered markdown.
//
// renderMarkdown(text, { tasks: true }) emits enabled checkboxes with the
// `md-task` class; taskToggleFromClick maps a click on the Nth checkbox
// back to the Nth task marker in the source and returns the flipped text.
// The source scan mirrors marked's task rule (bullet, `[ ]`/`[x]`, space,
// content) and skips fenced code. If the scan and the rendered DOM ever
// disagree on the count, the toggle is refused rather than flipping the
// wrong box.

export const TASK_CLASS = 'md-task'

const FENCE = /^ {0,3}(`{3,}|~{3,})/
const TASK = /^((?:[ \t]*>[ \t]?)*[ \t]*(?:[-*+]|\d{1,9}[.)])[ \t]+)\[([ xX])\](?=[ \t]+\S)/

interface TaskMarker {
  /** Offset of the character inside the brackets. */
  offset: number
  checked: boolean
}

export function findTasks(text: string): TaskMarker[] {
  const tasks: TaskMarker[] = []
  let fence: string | null = null
  let lineStart = 0
  for (const line of text.split('\n')) {
    const f = line.match(FENCE)
    if (fence) {
      if (f && f[1][0] === fence[0] && f[1].length >= fence.length) fence = null
    } else if (f) {
      fence = f[1]
    } else {
      const m = line.match(TASK)
      if (m) tasks.push({ offset: lineStart + m[1].length + 1, checked: m[2] !== ' ' })
    }
    lineStart += line.length + 1
  }
  return tasks
}

/** Flip the index-th task marker. Null when there is no such task. */
export function toggleTask(text: string, index: number): string | null {
  const task = findTasks(text)[index]
  if (!task) return null
  return text.slice(0, task.offset) + (task.checked ? ' ' : 'x') + text.slice(task.offset + 1)
}

/**
 * Click handler helper for a container holding rendered markdown. Returns
 * the toggled source when the click landed on a task checkbox (and claims
 * the event so the container doesn't enter edit mode), otherwise null.
 * The checkbox itself is left unchanged — it re-renders from the saved text.
 */
export function taskToggleFromClick(e: MouseEvent, source: string): string | null {
  const box = (e.target as Element | null)?.closest?.(`input.${TASK_CLASS}`)
  const root = e.currentTarget as Element | null
  if (!box || !root) return null
  e.preventDefault()
  e.stopPropagation()
  const boxes = Array.from(root.querySelectorAll(`input.${TASK_CLASS}`))
  const tasks = findTasks(source)
  const index = boxes.indexOf(box)
  if (boxes.length !== tasks.length || tasks[index]?.checked !== (box as HTMLInputElement).defaultChecked) return null
  return toggleTask(source, index)
}
