import { describe, it, expect } from 'vitest'
import { toolActionLabel } from '@shared/chatActionLabels'

// Echo translator: key + params, so assertions pin which key was chosen.
const t = (key: string, params?: Record<string, string | number>) =>
  params ? `${key} ${JSON.stringify(params)}` : key

describe('toolActionLabel', () => {
  it('labels native registry names and their legacy chat aliases alike', () => {
    expect(toolActionLabel({ tool: 'set_card_title', input: { title: 'A' } }, t))
      .toBe(toolActionLabel({ tool: 'set_title', input: { title: 'A' } }, t))
    expect(toolActionLabel({ tool: 'add_card_tags', input: { tags: ['x', 'y'] } }, t))
      .toBe('chat.action_added_tags {"tags":"x, y"}')
  })

  it('covers the tools that used to fall through to raw ids', () => {
    for (const tool of ['set_card_fields', 'add_card_blocks', 'pin_card', 'unpin_card', 'add_card_comment', 'create_category', 'update_cards', 'create_card_type', 'remove_card_tags']) {
      expect(toolActionLabel({ tool, input: {} }, t)).not.toBe(tool)
    }
  })

  it('distinguishes clearing from setting', () => {
    expect(toolActionLabel({ tool: 'set_card_due_date', input: { due_date: '' } }, t)).toBe('chat.action_due_date_cleared')
    expect(toolActionLabel({ tool: 'remove_card_tags', input: { all: true } }, t)).toBe('chat.action_removed_all_tags')
  })

  it('falls back to the tool id for unknown tools and survives malformed input', () => {
    expect(toolActionLabel({ tool: 'mystery', input: null }, t)).toBe('mystery')
    expect(toolActionLabel({ tool: 'add_card_tags', input: { tags: 'not-an-array' } }, t)).toBe('chat.action_added_tags {"tags":""}')
  })
})
