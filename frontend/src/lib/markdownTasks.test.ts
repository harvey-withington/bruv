import { describe, it, expect } from 'vitest'
import { findTasks, toggleTask, taskToggleFromClick } from '@shared/markdownTasks'
import { renderMarkdown } from '@shared/markdown'

const renderedBoxes = (md: string) =>
  (renderMarkdown(md, { tasks: true }).match(/<input [^>]*class="md-task"/g) ?? []).length

describe('toggleTask', () => {
  it('ticks and unticks the indexed task only', () => {
    const md = '- [ ] one\n- [x] two\n- [ ] three'
    expect(toggleTask(md, 0)).toBe('- [x] one\n- [x] two\n- [ ] three')
    expect(toggleTask(md, 1)).toBe('- [ ] one\n- [ ] two\n- [ ] three')
    expect(toggleTask(md, 3)).toBeNull()
  })

  it('handles nested, ordered, quoted and uppercase-X tasks', () => {
    const md = '1. [X] a\n   * [ ] b\n> - [ ] c'
    expect(findTasks(md).map((t) => t.checked)).toEqual([true, false, false])
    expect(toggleTask(md, 2)).toBe('1. [X] a\n   * [ ] b\n> - [x] c')
  })

  it('ignores fenced code and non-tasks', () => {
    const md = '```\n- [ ] code\n```\n- [ ]\n- [] nope\n- [ ] real'
    expect(findTasks(md)).toHaveLength(1)
    expect(toggleTask(md, 0)).toBe('```\n- [ ] code\n```\n- [ ]\n- [] nope\n- [x] real')
  })

  it.each([
    '- [ ] a\n- [x] b',
    '- [ ] a\n\n- [ ] loose',
    '1. [ ] a\n   - [x] nested',
    '> - [ ] quoted',
    '~~~\n- [ ] fenced\n~~~\n- [ ] after',
    'text\n- [ ] list after paragraph',
  ])('source scan agrees with rendered checkboxes: %j', (md) => {
    expect(findTasks(md)).toHaveLength(renderedBoxes(md))
  })
})

describe('taskToggleFromClick', () => {
  function clickBox(md: string, index: number) {
    const root = document.createElement('div')
    root.innerHTML = renderMarkdown(md, { tasks: true })
    let result: string | null = null
    root.addEventListener('click', (e) => { result = taskToggleFromClick(e, md) })
    root.querySelectorAll('input')[index].click()
    return result
  }

  it('maps the clicked checkbox to its source line', () => {
    expect(clickBox('- [ ] a\n- [ ] b', 1)).toBe('- [ ] a\n- [x] b')
  })

  it('refuses when the source no longer matches the rendered boxes', () => {
    const root = document.createElement('div')
    root.innerHTML = renderMarkdown('- [ ] a\n- [ ] b', { tasks: true })
    let result: string | null = 'unset'
    root.addEventListener('click', (e) => { result = taskToggleFromClick(e, '- [ ] a') })
    root.querySelectorAll('input')[1].click()
    expect(result).toBeNull()
  })

  it('ignores clicks off a checkbox', () => {
    const root = document.createElement('div')
    root.innerHTML = renderMarkdown('- [ ] a', { tasks: true })
    let result: string | null = 'unset'
    root.addEventListener('click', (e) => { result = taskToggleFromClick(e, '- [ ] a') })
    root.querySelector('li')!.click()
    expect(result).toBeNull()
  })
})
