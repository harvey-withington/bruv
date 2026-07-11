import { describe, it, expect } from 'vitest'
import { cardToMarkdown } from '@shared/cardMarkdown'
import type { Card, Block, CardComment } from '@shared/types'

function baseCard(overrides: Partial<Card> = {}): Card {
  return {
    id: 'c1',
    title: 'My Card',
    description: '',
    type: '',
    tags: [],
    due_date: null,
    created_at: '',
    blocks: [],
    file_attachments: [],
    ...overrides,
  }
}

describe('cardToMarkdown — frontmatter', () => {
  it('omits frontmatter entirely when nothing is set', () => {
    const md = cardToMarkdown(baseCard())
    expect(md).not.toContain('---\n')
    expect(md).toMatch(/^# My Card/)
  })

  it('includes only populated fields', () => {
    const md = cardToMarkdown(baseCard({
      type: 'task',
      tags: ['urgent', 'design'],
      due_date: '2026-06-01',
      created_at: '2026-05-24T10:00:00Z',
      members: ['Alice'],
    }))
    expect(md).toMatch(/^---\ntype: task\ntags: \[urgent, design\]\ndue: 2026-06-01\ncreated: 2026-05-24\nmembers: \[Alice\]\n---/)
  })

  it('quotes scalars with YAML-special characters', () => {
    const md = cardToMarkdown(baseCard({ tags: ['needs: review', 'q1'] }))
    expect(md).toContain('tags: ["needs: review", q1]')
  })
})

describe('cardToMarkdown — title and description', () => {
  it('falls back to untitledLabel when title is blank', () => {
    const md = cardToMarkdown(baseCard({ title: '   ' }), { untitledLabel: 'No title' })
    expect(md).toContain('# No title')
  })

  it('places description verbatim under the title', () => {
    const md = cardToMarkdown(baseCard({ description: 'A **bold** intro.' }))
    expect(md).toContain('# My Card\n\nA **bold** intro.\n')
  })
})

describe('cardToMarkdown — blocks', () => {
  function block(b: Partial<Block> & Pick<Block, 'type' | 'value'>): Block {
    return { id: 'b', label: '', key: '', meta: undefined, ...b } as Block
  }

  it('skips blocks with empty values (except divider and alarm)', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [
        block({ type: 'text', label: 'Notes', value: '' }),
        block({ type: 'list', label: 'Items', value: [] }),
      ],
    }))
    expect(md).not.toContain('## Notes')
    expect(md).not.toContain('## Items')
  })

  it('renders checklists with GFM task syntax', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({
        type: 'checklist',
        label: 'Tasks',
        value: [
          { id: '1', text: 'Write code', done: true },
          { id: '2', text: 'Ship it', done: false },
        ],
      })],
    }))
    expect(md).toContain('## Tasks')
    expect(md).toContain('- [x] Write code')
    expect(md).toContain('- [ ] Ship it')
  })

  it('renders lists as bullet items', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({
        type: 'list',
        label: 'Notes',
        value: [{ id: '1', text: 'one' }, { id: '2', text: 'two' }],
      })],
    }))
    expect(md).toContain('- one\n- two')
  })

  it('renders url block as a markdown link with caption fallback', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [
        block({ type: 'url', label: 'Link', value: { url: 'https://x.com', caption: 'X' } }),
        block({ type: 'url', label: 'Bare', value: { url: 'https://y.com' } }),
      ],
    }))
    expect(md).toContain('[X](https://x.com)')
    expect(md).toContain('[https://y.com](https://y.com)')
  })

  it('renders media as markdown images', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({
        type: 'media',
        label: 'Gallery',
        value: [{ id: '1', url: 'https://example.com/a.png', caption: 'A' }],
      })],
    }))
    expect(md).toContain('![A](https://example.com/a.png)')
  })

  it('renders rating as n / max', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({ type: 'rating', label: 'Score', value: 3, meta: { max: 5 } })],
    }))
    expect(md).toContain('3 / 5')
  })

  it('renders checkbox_group preserving option order', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({
        type: 'checkbox_group',
        label: 'Channels',
        value: ['email', 'sms'],
        meta: { options: ['email', 'sms', 'push'] },
      })],
    }))
    expect(md).toContain('- [x] email\n- [x] sms\n- [ ] push')
  })

  it('renders divider as horizontal rule', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({ type: 'divider', value: null })],
    }))
    expect(md).toMatch(/\n---\n/)
  })

  it('renders alarm metadata even with empty value', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({
        type: 'alarm',
        label: 'Reminder',
        value: null,
        meta: { alarm_time: '2026-06-01T09:00:00Z', alarm_fired: true },
      })],
    }))
    expect(md).toContain('## Reminder')
    expect(md).toContain('Alarm: 2026-06-01T09:00:00Z (fired)')
  })

  it('falls back to prettified key when label is blank', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({ type: 'text', label: '', key: 'team_notes', value: 'hi' })],
    }))
    expect(md).toContain('## Team Notes')
  })

  it('renders zero-valued number and progress blocks (0 is data, not empty)', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [
        block({ type: 'number', label: 'Count', value: 0 }),
        block({ type: 'progress', label: 'Done', value: 0, meta: { suffix: '%' } }),
      ],
    }))
    expect(md).toContain('## Count\n\n0')
    expect(md).toContain('## Done\n\n0 %')
  })

  it('renders unchecked checkbox blocks', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({ type: 'checkbox', label: 'Signed off', value: false })],
    }))
    expect(md).toContain('## Signed off')
    expect(md).toContain('- [ ]')
  })

  it('renders rating 0 (0 is data, not empty)', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [block({ type: 'rating', label: 'Score', value: 0, meta: { max: 5 } })],
    }))
    expect(md).toContain('## Score')
    expect(md).toContain('0 / 5')
  })

  it('still skips null/undefined-valued zero-meaningful blocks', () => {
    const md = cardToMarkdown(baseCard({
      blocks: [
        block({ type: 'number', label: 'Count', value: null }),
        block({ type: 'rating', label: 'Score', value: null }),
      ],
    }))
    expect(md).not.toContain('## Count')
    expect(md).not.toContain('## Score')
  })
})

describe('cardToMarkdown — localized labels', () => {
  it('uses provided section labels over the English defaults', () => {
    const md = cardToMarkdown(baseCard({
      file_attachments: [
        { id: '1', name: 'spec.pdf', path: 'p', mime: 'application/pdf', size: 100, added_at: '' },
      ],
    }), {
      comments: [{ id: '1', author: '', created_at: '2026-05-23T10:00:00Z', updated_at: '', text: 'hi' }],
      labels: { attachments: 'Anhänge', comments: 'Kommentare', unknownAuthor: 'Unbekannt' },
    })
    expect(md).toContain('## Anhänge')
    expect(md).toContain('## Kommentare')
    expect(md).toContain('### Unbekannt —')
    expect(md).not.toContain('## Attachments')
  })
})

describe('cardToMarkdown — attachments and comments', () => {
  it('lists attachment names without embedding bytes', () => {
    const md = cardToMarkdown(baseCard({
      file_attachments: [
        { id: '1', name: 'spec.pdf', path: 'p', mime: 'application/pdf', size: 100, added_at: '' },
      ],
    }))
    expect(md).toContain('## Attachments')
    expect(md).toContain('- spec.pdf')
  })

  it('appends a Comments section when comments are provided', () => {
    const comments: CardComment[] = [
      {
        id: '1', author: 'Alice',
        created_at: '2026-05-23T10:00:00Z', updated_at: '2026-05-23T10:00:00Z',
        text: 'Looks good.',
      },
    ]
    const md = cardToMarkdown(baseCard(), { comments })
    expect(md).toContain('## Comments')
    expect(md).toContain('### Alice — 2026-05-23')
    expect(md).toContain('Looks good.')
  })

  it('omits Comments section when none provided', () => {
    const md = cardToMarkdown(baseCard())
    expect(md).not.toContain('## Comments')
  })
})

describe('cardToMarkdown — output shape', () => {
  it('ends with exactly one trailing newline', () => {
    const md = cardToMarkdown(baseCard())
    expect(md.endsWith('\n')).toBe(true)
    expect(md.endsWith('\n\n')).toBe(false)
  })

  it('collapses runs of blank lines', () => {
    const md = cardToMarkdown(baseCard({
      description: 'desc',
      blocks: [
        { id: 'b1', type: 'text', label: 'A', key: '', value: 'a' },
        { id: 'b2', type: 'text', label: 'B', key: '', value: 'b' },
      ],
    }))
    expect(md).not.toMatch(/\n{3,}/)
  })
})
