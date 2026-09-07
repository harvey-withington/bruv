import { describe, it, expect } from 'vitest'
import { classifyFountain, fountainEntities, fountainOutline, fountainWordCount, renderFountain, renderFountainInline, type FountainLineKind } from '@shared/documentFormats/fountain'
import { documentFormatForPath } from '@shared/documentFormats'

const kinds = (text: string): FountainLineKind[] => classifyFountain(text).lines.map(l => l.kind)

describe('classifyFountain — the fountain.io syntax', () => {
  it('reads the title page until the first blank line, with multi-line values', () => {
    const doc = classifyFountain('Title:\n    _**BRICK & STEEL**_\n    _**FULL RETIRED**_\nCredit: Written by\nAuthor: Stu Maschwitz\nDraft date: 1/20/2012\n\nEXT. BRICK\'S PATIO - DAY\n\nA gorgeous day.')
    expect(doc.titlePage).toEqual([
      { key: 'Title', value: '_**BRICK & STEEL**_\n_**FULL RETIRED**_' },
      { key: 'Credit', value: 'Written by' },
      { key: 'Author', value: 'Stu Maschwitz' },
      { key: 'Draft date', value: '1/20/2012' },
    ])
    expect(doc.lines.slice(0, 6).map(l => l.kind)).toEqual(Array<FountainLineKind>(6).fill('title_page'))
    expect(doc.lines[7].kind).toBe('scene_heading')
  })

  it('does not mistake a colon in the opening action for a title page', () => {
    expect(kinds('Warning: this is action\n\nINT. HOUSE - DAY\n')).toEqual(['action', 'blank', 'scene_heading', 'blank'])
    // "Notes:" is a real title-page key, so it does open one.
    expect(kinds('Notes: first draft\n\nINT. HOUSE - DAY\n')).toEqual(['title_page', 'blank', 'scene_heading', 'blank'])
  })

  it('classifies scene headings by prefix, needing blank lines around them', () => {
    expect(kinds('INT. HOUSE - DAY\n\nAction.')).toEqual(['scene_heading', 'blank', 'action'])
    expect(kinds('ext. beach - night\n\nAction.')).toEqual(['scene_heading', 'blank', 'action'])
    expect(kinds('I/E. CAR - CONTINUOUS\n\nAction.')).toEqual(['scene_heading', 'blank', 'action'])
    expect(kinds('EST. CITY\n\nAction.')).toEqual(['scene_heading', 'blank', 'action'])
    // No blank line after: still a heading (never a character named
    // "INT. HOUSE"), so the line doesn't flicker while being typed.
    expect(kinds('INT. HOUSE - DAY\nAction right after.')).toEqual(['scene_heading', 'action'])
    // Not at a block start: action.
    expect(kinds('He said\nINT. HOUSE - DAY')).toEqual(['action', 'action'])
    // A word that merely starts with "int" is not a heading.
    expect(kinds('Interior thoughts.\n\n')).toEqual(['action', 'blank', 'blank'])
  })

  it('forces a scene heading with a leading period and extracts the scene number', () => {
    const doc = classifyFountain('.SNIPER SCOPE POV #1A#\n\nAction.')
    expect(doc.lines[0].kind).toBe('scene_heading')
    expect(doc.lines[0].text).toBe('SNIPER SCOPE POV')
    expect(doc.lines[0].sceneNumber).toBe('1A')
    expect(doc.lines[0].forced).toBe(true)
    // An ellipsis is action, not a forced heading.
    expect(kinds('...and then.\n')).toEqual(['action', 'blank'])
  })

  it('finds character cues, parentheticals and dialogue', () => {
    const doc = classifyFountain('STEEL\nThe man\'s a wonder.\n\nBRICK (V.O.)\n(beat)\nYes.\n\nMcCLANE\nNope.')
    expect(doc.lines.map(l => l.kind)).toEqual([
      'character', 'dialogue', 'blank', 'character', 'parenthetical', 'dialogue', 'blank', 'action', 'action',
    ])
    expect(doc.lines[3].text).toBe('BRICK (V.O.)')
  })

  it('does not treat an uppercase line with nothing after it as a cue', () => {
    expect(kinds('THE END\n\n')).toEqual(['action', 'blank', 'blank'])
    expect(kinds('Action.\nSTEEL\nHello.')).toEqual(['action', 'action', 'action'])
  })

  it('forces a cue with @ and marks dual dialogue with ^', () => {
    const doc = classifyFountain('@McCLANE\nYippee.\n\nBRICK\nScrew retirement.\n\nSTEEL ^\nScrew retirement.')
    expect(doc.lines[0].kind).toBe('character')
    expect(doc.lines[0].text).toBe('McCLANE')
    expect(doc.lines[0].forced).toBe(true)
    expect(doc.lines[6].kind).toBe('character')
    expect(doc.lines[6].dual).toBe(true)
    expect(doc.lines[6].text).toBe('STEEL')
    expect(doc.lines[3].dual).toBeUndefined()
  })

  it('keeps a dialogue block open across a two-space line', () => {
    expect(kinds('DEALER\nTen.\n  \nFour.')).toEqual(['character', 'dialogue', 'dialogue', 'dialogue'])
    expect(kinds('DEALER\nTen.\n\nFour.')).toEqual(['character', 'dialogue', 'blank', 'action'])
  })

  it('classifies transitions: uppercase ending in TO:, or forced with >', () => {
    expect(kinds('Action.\n\nCUT TO:\n\nINT. X\n\n')).toEqual(['action', 'blank', 'transition', 'blank', 'scene_heading', 'blank', 'blank'])
    expect(kinds('> Burn to White.\n')).toEqual(['transition', 'blank'])
    expect(classifyFountain('> Burn to White.').lines[0].text).toBe('Burn to White.')
    // Not surrounded by blanks → action.
    expect(kinds('Action.\nCUT TO:\n\n')).toEqual(['action', 'action', 'blank', 'blank'])
  })

  it('classifies centered text, page breaks, sections, synopses, lyrics and forced action', () => {
    const doc = classifyFountain('> THE END <\n===\n# Act One\n## Sequence 1\n= Brick meets Steel.\n~Willy Wonka! Willy Wonka!\n!STEEL is not a cue')
    expect(doc.lines.map(l => l.kind)).toEqual(['centered', 'page_break', 'section', 'section', 'synopsis', 'lyric', 'action'])
    expect(doc.lines[0].text).toBe('THE END')
    expect(doc.lines[2].level).toBe(1)
    expect(doc.lines[3].level).toBe(2)
    expect(doc.lines[3].text).toBe('Sequence 1')
    expect(doc.lines[4].text).toBe('Brick meets Steel.')
    expect(doc.lines[6].forced).toBe(true)
    expect(doc.lines[6].text).toBe('STEEL is not a cue')
  })

  it('treats whole-line notes and boneyard as their own kinds, and as blanks for context', () => {
    const doc = classifyFountain('STEEL\nHi.\n[[Rewrite this]]\nNot dialogue.\n\n/* old\ndraft */\n\nINT. X\n\n')
    expect(doc.lines.map(l => l.kind)).toEqual([
      'character', 'dialogue', 'note', 'action', 'blank', 'boneyard', 'boneyard', 'blank', 'scene_heading', 'blank', 'blank',
    ])
    // A multi-line note with text on the closing line is still a note.
    expect(kinds('[[one\ntwo]]\n')).toEqual(['note', 'note', 'blank'])
    // A note that shares its line with text leaves the line's kind alone.
    expect(kinds('He walks in. [[check]]\n')).toEqual(['action', 'blank'])
  })

  it('reports document offsets per line', () => {
    const doc = classifyFountain('AB\nCDE')
    expect(doc.lines.map(l => [l.from, l.to])).toEqual([[0, 2], [3, 6]])
  })

  it('handles CRLF and an empty document', () => {
    expect(kinds('INT. X\r\n\r\nGo.')).toEqual(['scene_heading', 'blank', 'action'])
    expect(classifyFountain('').lines).toEqual([{ line: 1, kind: 'blank', raw: '', text: '', from: 0, to: 0 }])
  })
})

describe('renderFountain', () => {
  it('renders the title page, headings, dialogue and transitions with the screenplay classes', () => {
    const html = renderFountain('Title: BRICK\nAuthor: Stu\n\nINT. HOUSE - DAY #3#\n\nA gorgeous day. <script>\n\nBRICK\n(beat)\nYes.\n\nCUT TO:\n')
    expect(html).toContain('<section class="fn-title-page">')
    expect(html).toContain('<p class="fn-tp fn-tp-title">BRICK</p>')
    expect(html).toContain('<p class="fn-scene-heading">INT. HOUSE - DAY<span class="fn-scene-number">3</span></p>')
    expect(html).toContain('<p class="fn-action">A gorgeous day. &lt;script&gt;</p>')
    expect(html).toContain('<div class="fn-dialogue-block"><p class="fn-character">BRICK</p><p class="fn-parenthetical">(beat)</p><p class="fn-dialogue">Yes.</p></div>')
    expect(html).toContain('<p class="fn-transition">CUT TO:</p>')
  })

  it('pairs dual dialogue into one row and keeps action line breaks', () => {
    const html = renderFountain('BRICK\nOne.\n\nSTEEL ^\nTwo.\n\nThey run.\nThey jump.')
    expect(html).toContain('<div class="fn-dual"><div class="fn-dialogue-block"><p class="fn-character">BRICK</p><p class="fn-dialogue">One.</p></div><div class="fn-dialogue-block"><p class="fn-character">STEEL</p><p class="fn-dialogue">Two.</p></div></div>')
    expect(html).toContain('<p class="fn-action">They run.<br>They jump.</p>')
  })

  it('renders emphasis, keeps notes muted and drops boneyard', () => {
    expect(renderFountainInline('***all*** **bold** *it* _under_ \\*lit\\* [[note]] /* gone */ a & b'))
      .toBe('<strong><em>all</em></strong> <strong>bold</strong> <em>it</em> <u>under</u> &#42;lit&#42; <span class="fn-note">note</span>  a &amp; b')
  })

  it('renders page breaks, centered text, sections and synopses', () => {
    const html = renderFountain('> THE END <\n===\n# Act One\n= Summary')
    expect(html).toContain('<p class="fn-centered">THE END</p>')
    expect(html).toContain('<hr class="fn-page-break">')
    expect(html).toContain('<p class="fn-section fn-section-1">Act One</p>')
    expect(html).toContain('<p class="fn-synopsis">Summary</p>')
  })
})

describe('fountain outline, entities, word count', () => {
  const script = '# Act One\n= The setup.\n\nINT. HOUSE - DAY #1#\n= Brick wakes.\n\nBRICK\nMorning.\n\nSTEEL (V.O.)\nHi.\n\nBRICK (CONT\'D)\nStill morning.\n\n## Chase\n\nEXT. STREET - DAY\n\n[[a note]]\n\nCUT TO:\n'

  it('nests scene headings under the current section and attaches synopses', () => {
    expect(fountainOutline(script)).toEqual([
      { id: '1', level: 1, title: 'Act One', line: 1, description: 'The setup.' },
      { id: '4', level: 2, title: '1. INT. HOUSE - DAY', line: 4, description: 'Brick wakes.' },
      { id: '16', level: 2, title: 'Chase', line: 16 },
      { id: '18', level: 3, title: 'EXT. STREET - DAY', line: 18 },
    ])
  })

  it('dedupes characters by bare name and counts their dialogue blocks', () => {
    expect(fountainEntities(script)).toEqual([
      { kind: 'character', name: 'BRICK', count: 2 },
      { kind: 'character', name: 'STEEL', count: 1 },
    ])
  })

  it('counts only the words the reader sees', () => {
    // Headings (3 + 3) + cues (1 + 2 + 2) + dialogue (1 + 1 + 2) + transition (2) = 17;
    // dashes, sections, synopses and the note don't count.
    expect(fountainWordCount(script)).toBe(17)
  })

  it('is registered for .fountain files', () => {
    expect(documentFormatForPath('drafts/pilot.fountain').id).toBe('fountain')
  })
})
