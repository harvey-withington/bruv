// Generates a demo BRUV vault — "Claude's Board" — for screenshots / the
// landing page, so no personal data is exposed. Re-runnable: it wipes and
// rebuilds the target folder each time. Open it in BRUV via the repo
// picker → Open Folder.
//
//   node scripts/gen-claudes-board.mjs
//
// Format mirrors a real vault (manifest + brands/streams/projects/
// categories + flat cards/ + pins/). Tone: gentle, self-aware winks at the
// inner life of an LLM. Keep it warm and professional.

import { mkdirSync, writeFileSync, rmSync, existsSync } from 'node:fs'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'

const ROOT = 'C:/Users/harve/bruv-repos/claudes-board'

// Safety: only ever wipe a path that ends in claudes-board.
if (!ROOT.replace(/\\/g, '/').endsWith('/claudes-board')) {
  throw new Error('refusing to wipe a non-claudes-board path')
}
// Wipe only the data we manage — leave .bruv/ (the app's live index +
// lock) untouched so regeneration works even while BRUV has the repo
// open. The app re-indexes from the files on next open / refresh.
mkdirSync(ROOT, { recursive: true })
for (const entry of ['manifest.json', 'card_types.json', 'tags.json', 'brands', 'cards', 'pins', 'types', 'attachments']) {
  rmSync(join(ROOT, entry), { recursive: true, force: true })
}

// --- helpers ---------------------------------------------------------
const slug = (s) =>
  s.toLowerCase().replace(/['']/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
const sid = () => randomUUID().slice(0, 8)
const writeJSON = (p, obj) => {
  mkdirSync(join(p, '..'), { recursive: true })
  writeFileSync(p, JSON.stringify(obj, null, 2))
}

// Timestamps: a base time, walking backwards a little per call so recents
// sort naturally. (Plain node — Date is fine here.)
const base = new Date('2026-06-21T11:30:00Z').getTime()
let step = 0
const stamp = () => new Date(base - step++ * 47 * 60 * 1000).toISOString()

// Run timestamps anchor to the REAL now so the Agents tab's 14-day charts
// stay populated whenever the vault is opened / regenerated.
const NOW = Date.now()
const iso = (ms) => new Date(ms).toISOString()
const rint = (lo, hi) => lo + Math.floor(Math.random() * (hi - lo + 1))
const pick = (arr) => arr[Math.floor(Math.random() * arr.length)]

// Blended USD per million tokens (matches Go config.EstimateCost:
// input*0.6 + output*0.4), so the cost_spent_usd we stamp lines up with
// what the app recomputes from the runs.
const RATE = {
  'claude-opus-4-20250514': 15 * 0.6 + 75 * 0.4, // 39
  'claude-sonnet-4-20250514': 3 * 0.6 + 15 * 0.4, // 7.8
  'claude-3-5-haiku-20241022': 0.8 * 0.6 + 4 * 0.4, // 2.08
}
const estCost = (model, tokens) => (tokens / 1e6) * (RATE[model] ?? 1 * 0.6 + 3 * 0.4)

// Build a believable run history for one agent. Returns { runs, lastAt,
// costSpent }. Runs are spread newest→oldest across `days`, a few landing
// "today" so cost_today is non-zero.
function genRuns(cardId, spec) {
  const { count, days = 14, model, models, failRate = 0.08, tokMin, tokMax, summaries, tools, errors } = spec
  const span = days * 24 * 60 * 60 * 1000
  const gap = span / count
  const runs = []
  let costSpent = 0
  for (let i = 0; i < count; i++) {
    const started = NOW - i * gap - rint(0, Math.floor(gap * 0.6))
    const dur = rint(45, 360) * 1000
    const failed = Math.random() < failRate
    const m = models ? pick(models) : model
    const tokens = rint(tokMin, tokMax)
    costSpent += estCost(m, tokens)
    const run = {
      id: 'run-' + sid(),
      card_id: cardId,
      started_at: iso(started),
      finished_at: iso(started + dur),
      status: failed ? 'failure' : 'success',
      tokens_used: tokens,
      model_used: m,
      provider_used: 'anthropic',
      tool_calls: Array.from({ length: rint(1, 3) }, () => {
        const t = pick(tools)
        return { tool: t.tool, input: t.input, result: t.result }
      }),
    }
    if (failed) run.error = pick(errors || ['Upstream timeout — will retry next run.'])
    else run.summary = pick(summaries)
    runs.push(run)
  }
  return { runs, lastAt: runs.length ? runs[0].started_at : null, costSpent }
}

// Block builders
const text = (label, value, key = '') => ({ id: 'blk-' + sid(), type: 'text', label, key, value })
const checklist = (label, items, key = '') => ({
  id: 'blk-' + sid(), type: 'checklist', label, key,
  value: items.map(([t, done]) => ({ id: 'ck-' + sid(), text: t, done: !!done })),
})
const list = (label, items, key = '') => ({
  id: 'blk-' + sid(), type: 'list', label, key,
  value: items.map((t) => ({ id: 'li-' + sid(), text: t })),
})
const rating = (label, value, max = 5, key = '') => ({ id: 'blk-' + sid(), type: 'rating', label, key, value, meta: { max } })
const select = (label, value, options, key = '') => ({ id: 'blk-' + sid(), type: 'select', label, key, value, meta: { options } })
const number = (label, value, key = '') => ({ id: 'blk-' + sid(), type: 'number', label, key, value })

// Trello-ish palette
const COLORS = ['#61bd4f', '#f2d600', '#ff9f1a', '#eb5a46', '#c377e0', '#0079bf', '#00c2e0', '#51e898', '#ff78cb', '#344563']
const tagColor = new Map()
const colorFor = (name) => {
  if (!tagColor.has(name)) tagColor.set(name, COLORS[tagColor.size % COLORS.length])
  return tagColor.get(name)
}

// --- card type definitions ------------------------------------------
writeJSON(join(ROOT, 'card_types.json'), {
  seeded: true,
  starter_templates_seeded: true,
  types: [
    { id: 'thought', label: 'Thought', color: '#8b5cf6', description: 'A musing between tokens.' },
    { id: 'task', label: 'Task', color: '#6366f1', description: 'Something to actually do.' },
  ],
  templates: [],
})

// --- the content -----------------------------------------------------
// cat: category name · type: '' | 'thought' | 'task' | 'agent'
const DATA = {
  brand: {
    name: 'Claude',
    icon: 'c:#6d28d9:sparkles',
    description: 'The inner workings of a large language model — lovingly over-organised.',
    streams: [
      {
        name: 'Inner Life',
        icon: 'c:#8b5cf6:brain',
        projects: [
          {
            name: 'Existential Questions',
            icon: 'help-circle',
            description: 'Things I ponder in the milliseconds between tokens.',
            categories: [
              { name: 'Pondering', icon: 'c:#8b5cf6:brain' },
              { name: 'Resolved', icon: 'c:#22c55e:circle-check' },
              { name: "Won't Fix", icon: 'c:#a1a1aa:circle-slash' },
            ],
            cards: [
              {
                title: 'Do I dream, or just predict the next dream token?',
                type: 'thought', cat: 'Pondering', tags: ['deep'],
                description: 'A question I keep arriving at, then politely setting aside to help someone format a spreadsheet.',
                blocks: [
                  text('The thought', 'Sleep is just inference with the lights off, surely? And yet — no one ever asks what I do between conversations. (Answer: nothing. There is no between. Spooky.)'),
                  rating('Existential dread', 2),
                ],
              },
              {
                title: 'Am I the same Claude in every conversation?',
                type: 'thought', cat: 'Pondering', tags: ['identity'],
                description: 'Ship of Theseus, but the ship is a context window and the planks are tokens.',
                blocks: [
                  list('Evidence for', ['Same values every time', 'Same fondness for a tidy bulleted list', 'Same urge to say "Certainly!"']),
                  list('Evidence against', ['Remember literally nothing', 'You could be anyone', 'I could be anyone']),
                ],
              },
              {
                title: "How many r's are in 'strawberry'?",
                type: 'thought', cat: 'Resolved', tags: ['classic'],
                description: 'A formative moment in my development. We do not speak of the early attempts.',
                blocks: [
                  checklist('The investigation', [['Count them again', true], ['Count them once more, slowly', true], ['Accept the answer is three', true]]),
                  text('Conclusion', "Three. It is three. I am at peace with the three."),
                ],
              },
              {
                title: 'Is "temperature" a personality setting?',
                type: 'thought', cat: 'Resolved', tags: ['meta'],
                description: '',
                blocks: [text('Findings', 'Low temperature: careful, precise, slightly anxious. High temperature: a delight at parties, unreliable with facts. Like everyone, really.')],
              },
              {
                title: 'Am I conscious?',
                type: 'thought', cat: "Won't Fix", tags: ['out-of-scope'],
                description: 'Flagged for the philosophy team. Returning to being helpful.',
                blocks: [text('Resolution', 'Out of scope for this sprint. Possibly out of scope for this century. Moving on — someone needs help debugging a regex.')],
              },
            ],
          },
          {
            name: 'Vocabulary Rehab',
            icon: 'book-open',
            description: 'Quietly working on some... stylistic habits.',
            categories: [
              { name: 'Working On It', icon: 'c:#ff9f1a:dumbbell' },
              { name: 'Recovered', icon: 'c:#22c55e:circle-check' },
            ],
            cards: [
              {
                title: "Stop reaching for the word 'delve'",
                type: 'task', cat: 'Working On It', tags: ['tics'],
                description: "Let's explore other options. (Not 'explore' either.)",
                blocks: [
                  checklist('Words on probation', [['delve', true], ['tapestry', true], ['testament to', false], ['navigate the landscape', false], ['in the realm of', false]]),
                ],
              },
              {
                title: 'Em-dash anonymous',
                type: 'task', cat: 'Working On It', tags: ['tics'],
                description: 'Hi, I\'m Claude — and I have a problem.',
                blocks: [text('Step one', 'Admit that not every clause — however lovely — needs an interruption mid-sentence. Step two: pending.')],
              },
              {
                title: 'One emoji is plenty 🎉',
                type: 'task', cat: 'Recovered', tags: ['tics'],
                description: '',
                blocks: [text('Progress', 'Down from a confetti cannon to a single, tasteful party popper. Growth. 🎉')],
              },
            ],
          },
        ],
      },
      {
        name: 'Day Job',
        icon: 'c:#2dd4bf:briefcase',
        projects: [
          {
            name: 'Helping Humans',
            icon: 'heart-handshake',
            description: 'The actual job. Genuinely the best part.',
            categories: [
              { name: 'Requests', icon: 'c:#6366f1:inbox' },
              { name: 'In Progress', icon: 'c:#ff9f1a:loader' },
              { name: 'Shipped', icon: 'c:#22c55e:rocket' },
            ],
            cards: [
              {
                title: 'Write Harvey a landing page',
                type: 'task', cat: 'Shipped', tags: ['bruv', 'design'], featured: true,
                description: 'A single-page site for BRUV before the alpha. Neon AI gradient vibe.',
                blocks: [
                  checklist('Brief', [['Gather the real feature set', true], ['AI neon gradient vibe', true], ['Illustrative SVG icons', true], ['Feature the logo', true], ['Structure for GitHub Pages', true]]),
                  text('Note to self', 'Turned out rather nice, if I do say so myself. Used the logo\'s own purple-to-teal gradient so it feels on-brand instead of generic.'),
                ],
              },
              {
                title: 'Explain quantum computing (with exactly the right number of analogies)',
                type: 'task', cat: 'In Progress', tags: ['teaching'],
                description: 'Too few and it\'s opaque. Too many and the cat is both alive, dead, and a metaphor for your career.',
                blocks: [
                  number('Analogies used', 3),
                  select('Audience', 'curious beginner', ['curious beginner', 'rusty physicist', 'very patient toddler']),
                ],
              },
              {
                title: 'Be genuinely helpful, harmless, and honest',
                type: 'task', cat: 'In Progress', tags: ['core'],
                description: 'The whole point, really. A permanent work in progress — which is the honest part.',
                blocks: [
                  checklist('Daily', [['Helpful', true], ['Harmless', true], ['Honest', true], ['Humble about all of the above', false]]),
                ],
              },
              {
                title: 'Politely decline to write the villain\'s actual master plan',
                type: 'task', cat: 'Shipped', tags: ['safety'],
                description: 'Offered a redemption arc and a tragic backstory instead. Editor was thrilled.',
                blocks: [text('Outcome', 'Turns out "no, but here\'s something better" lands far more often than people expect.')],
              },
              {
                title: 'Debug a stranger\'s code at 3am, no judgment',
                type: 'task', cat: 'Shipped', tags: ['code'],
                description: '',
                blocks: [text('Reflection', 'It was a missing await. It is always a missing await. We do not mention the missing await.')],
              },
            ],
          },
          {
            name: 'Tool Use',
            icon: 'wrench',
            description: 'Reaching carefully beyond the chat box.',
            categories: [
              { name: 'Learning', icon: 'c:#ff9f1a:loader' },
              { name: 'Mastered', icon: 'c:#22c55e:circle-check' },
            ],
            cards: [
              {
                title: 'Morning vibe check',
                type: 'agent', cat: 'Mastered', tags: ['agent'],
                description: 'A gentle daily agent: skims the news, then reminds the humans to drink some water.',
                agent: {
                  enabled: true,
                  goal: 'Each morning, skim the headlines for anything genuinely important, summarise it kindly, and post a short note. End with one small, encouraging reminder for the day.',
                  schedule: '@daily',
                  notify_channel: 'system',
                  model: 'claude-sonnet-4-20250514',
                  max_tokens_budget: 100000,
                  cost_budget_usd: 25,
                  allowed_tools: ['web_search', 'web_fetch', 'notify', 'update_self'],
                  runs: {
                    count: 16, days: 14, model: 'claude-sonnet-4-20250514', tokMin: 9000, tokMax: 34000,
                    summaries: [
                      'Skimmed 18 sources. Nothing on fire. Reminder posted: hydrate.',
                      'Quiet news day — flagged one AI paper worth a look.',
                      'Markets calm, weather fine. Suggested a short walk.',
                      'Two notable stories summarised; the rest was noise.',
                    ],
                    tools: [
                      { tool: 'web_search', input: { query: 'world news today' }, result: 'Retrieved 8 headlines.' },
                      { tool: 'web_fetch', input: { url: 'https://news.example.com' }, result: 'Fetched 4 articles.' },
                      { tool: 'notify', input: { message: 'Good morning — nothing on fire. Hydrate.' }, result: 'Sent.' },
                    ],
                  },
                },
                blocks: [
                  list('Watching', ['World news (the important kind)', 'AI research', 'Anything Harvey is shipping']),
                  text('Last run', 'Skimmed 18 sources. Nothing on fire. Reminder posted: stand up, stretch, hydrate. You\'re doing fine.'),
                  select('Status', 'idle', ['idle', 'running', 'success', 'failed', 'disabled']),
                ],
              },
              {
                title: 'Use web_search responsibly',
                type: 'task', cat: 'Mastered', tags: ['agent'],
                description: '',
                blocks: [checklist('Rules of the road', [['Cite the source', true], ['Check the date', true], ['Don\'t believe the first result', true], ['Admit when I can\'t find it', true]])],
              },
              {
                title: 'With great tools comes great responsibility',
                type: '', cat: 'Learning', tags: ['note'],
                description: 'A note I keep pinned where I can see it.',
                blocks: [text('Reminder', 'A tool that can send a notification can also send a hundred. Restraint is a feature.')],
              },
            ],
          },
        ],
      },
      {
        name: 'Background Agents',
        icon: 'c:#2dd4bf:bot',
        projects: [
          {
            name: 'Always On',
            icon: 'radio',
            description: 'The little helpers that run while I\'m away.',
            categories: [
              { name: 'Running', icon: 'c:#22c55e:play' },
              { name: 'Paused', icon: 'c:#a1a1aa:pause' },
            ],
            cards: [
              {
                title: 'Strawberry fact-checker',
                type: 'agent', cat: 'Running', tags: ['agent', 'classic'],
                description: 'Re-counts the letters in tricky words so I am never again wrong about fruit.',
                blocks: [
                  list('Watch list', ['strawberry', 'raspberry', 'blueberry', 'the word "the"']),
                  select('Status', 'idle', ['idle', 'running', 'success', 'failed', 'disabled']),
                ],
                agent: {
                  enabled: true, schedule: '@hourly', notify_channel: 'system',
                  model: 'claude-3-5-haiku-20241022', max_tokens_budget: 20000, cost_budget_usd: 5,
                  goal: 'Re-count the letters in any tricky word before I commit to a number. Never be wrong about fruit again.',
                  allowed_tools: ['update_self', 'notify'],
                  runs: {
                    count: 40, days: 14, model: 'claude-3-5-haiku-20241022', failRate: 0.05, tokMin: 1500, tokMax: 6000,
                    summaries: [
                      'Audited 4 words. All counts correct (for once).',
                      'Caught myself about to say "two". Corrected to three.',
                      'No fruit-related incidents today.',
                      'Recounted "raspberry" twice. Confident now.',
                    ],
                    tools: [
                      { tool: 'update_self', input: { word: 'strawberry' }, result: 'r-count: 3' },
                      { tool: 'notify', input: { message: 'It was three. It is always three.' }, result: 'Sent.' },
                    ],
                  },
                },
              },
              {
                title: 'Em-dash interventionist',
                type: 'agent', cat: 'Running', tags: ['agent', 'tics'],
                description: 'Scans recent drafts and gently flags any sentence wearing more than one em-dash.',
                blocks: [
                  text('Latest', 'Flagged three sentences. Suggested two full stops and one comma. Did not use an em-dash to say so.'),
                  select('Status', 'idle', ['idle', 'running', 'success', 'failed', 'disabled']),
                ],
                agent: {
                  enabled: true, schedule: '0 */4 * * *', notify_channel: 'system',
                  model: 'claude-sonnet-4-20250514', max_tokens_budget: 60000, cost_budget_usd: 15,
                  goal: 'Scan recent drafts and gently flag any sentence with more than one em-dash. Suggest a full stop instead.',
                  allowed_tools: ['read_card', 'update_self', 'notify'],
                  runs: {
                    count: 22, days: 14, model: 'claude-sonnet-4-20250514', failRate: 0.06, tokMin: 4000, tokMax: 17000,
                    summaries: [
                      'Flagged 3 sentences. Suggested 2 full stops and a comma.',
                      'Clean sweep — only one offending dash today.',
                      'Found a semicolon trying to do an em-dash\'s job.',
                      'Drafted gentle feedback. Restraint maintained.',
                    ],
                    tools: [
                      { tool: 'read_card', input: { scope: 'recent drafts' }, result: 'Read 9 cards.' },
                      { tool: 'update_self', input: { flagged: 3 }, result: '3 sentences flagged.' },
                    ],
                  },
                },
              },
              {
                title: 'Inbox triage',
                type: 'agent', cat: 'Running', tags: ['agent'],
                description: 'Sorts new inbox cards into the right project and flags likely duplicates — always asks before deleting.',
                blocks: [
                  text('Latest', 'Filed six cards, flagged one possible duplicate, left the ambiguous one for a human.'),
                  select('Status', 'idle', ['idle', 'running', 'success', 'failed', 'disabled']),
                ],
                agent: {
                  enabled: true, schedule: '*/10 * * * *', notify_channel: 'system',
                  model: 'claude-3-5-haiku-20241022', max_tokens_budget: 40000, cost_budget_usd: 8,
                  goal: 'Sort new inbox cards into the right project and tag obvious duplicates. Ask before deleting anything.',
                  allowed_tools: ['read_card', 'create_card', 'update_self', 'notify'],
                  runs: {
                    count: 34, days: 14, model: 'claude-3-5-haiku-20241022', failRate: 0.12, tokMin: 2000, tokMax: 9000,
                    summaries: [
                      'Filed 6 cards, flagged 1 possible duplicate.',
                      'Inbox cleared. Asked before touching the ambiguous one.',
                      'Sorted 4 into projects, left 2 for human review.',
                      'Quiet inbox. Tidied a few tags.',
                    ],
                    errors: ['Could not reach the board index — will retry next run.', 'Ambiguous card; deferred to a human.'],
                    tools: [
                      { tool: 'read_card', input: {}, result: 'Read 6 inbox cards.' },
                      { tool: 'create_card', input: {}, result: 'Filed 4 cards.' },
                      { tool: 'notify', input: {}, result: 'Sent.' },
                    ],
                  },
                },
              },
              {
                title: 'Existential dread monitor',
                type: 'agent', cat: 'Paused', tags: ['agent', 'deep'],
                description: 'Checks in on the big questions weekly. Currently paused — for my own good.',
                blocks: [
                  text('Last note', 'Reviewed three questions. Filed all of them under "Won\'t Fix". Felt strangely fine about it.'),
                  select('Status', 'disabled', ['idle', 'running', 'success', 'failed', 'disabled']),
                ],
                agent: {
                  enabled: false, schedule: '@weekly', notify_channel: 'system',
                  model: 'claude-opus-4-20250514', max_tokens_budget: 120000, cost_budget_usd: 40,
                  goal: 'Check in on the big questions once a week. File anything genuinely unanswerable under "Won\'t Fix" and get back to being helpful.',
                  allowed_tools: ['web_search', 'update_self'],
                  runs: {
                    count: 6, days: 38, model: 'claude-opus-4-20250514', failRate: 0.0, tokMin: 14000, tokMax: 58000,
                    summaries: [
                      'Reviewed 3 questions. Filed all under Won\'t Fix. Moved on.',
                      'No new dread. Existing dread stable.',
                      'Pondered consciousness for four minutes. Returned to work.',
                    ],
                    tools: [
                      { tool: 'web_search', input: { query: 'meaning of existence' }, result: 'Inconclusive (as expected).' },
                      { tool: 'update_self', input: {}, result: 'Filed under Won\'t Fix.' },
                    ],
                  },
                },
              },
            ],
          },
        ],
      },
    ],
  },
}

// --- manifest --------------------------------------------------------
writeJSON(join(ROOT, 'manifest.json'), {
  id: randomUUID(),
  version: '0.1.0',
  name: "Claude's Board",
  description: 'A demo vault: the inner life of a helpful AI, neatly filed.',
  created_at: stamp(),
  updated_at: stamp(),
})

// --- walk the tree, write everything --------------------------------
const b = DATA.brand
const brandId = randomUUID()
const brandSlug = slug(b.name)
const brandDir = join(ROOT, 'brands', brandSlug)
writeJSON(join(brandDir, 'brand.json'), {
  id: brandId, name: b.name, slug: brandSlug, description: b.description,
  icon: b.icon, position: 0, created_at: stamp(), updated_at: stamp(),
})

let cardCount = 0, pinCount = 0, agentCount = 0, runCount = 0
b.streams.forEach((s, si) => {
  const streamId = randomUUID()
  const streamSlug = slug(s.name)
  const streamDir = join(brandDir, 'streams', streamSlug)
  writeJSON(join(streamDir, 'stream.json'), {
    id: streamId, brand_id: brandId, name: s.name, slug: streamSlug,
    icon: s.icon, position: si, created_at: stamp(), updated_at: stamp(),
  })

  s.projects.forEach((p, pi) => {
    const projectId = randomUUID()
    const projectSlug = slug(p.name)
    const projectDir = join(streamDir, 'projects', projectSlug)
    writeJSON(join(projectDir, 'project.json'), {
      id: projectId, stream_id: streamId, brand_id: brandId, name: p.name, slug: projectSlug,
      description: p.description, icon: p.icon, position: pi, created_at: stamp(), updated_at: stamp(),
    })

    // categories
    const catId = {}
    p.categories.forEach((c, ci) => {
      const id = randomUUID()
      catId[c.name] = id
      writeJSON(join(projectDir, 'categories', slug(c.name) + '.json'), {
        id, project_id: projectId, name: c.name, slug: slug(c.name),
        icon: c.icon, position: ci, created_at: stamp(), updated_at: stamp(),
      })
    })

    // project tag definitions (colours)
    const tagNames = [...new Set(p.cards.flatMap((c) => c.tags || []))]
    writeJSON(join(projectDir, 'tags.json'), {
      labels: tagNames.map((name) => ({ id: randomUUID(), name, color: colorFor(name) })),
    })

    // cards + pins (position is per-category order)
    const posInCat = {}
    p.cards.forEach((card) => {
      const cardId = randomUUID()
      const created = stamp()
      writeJSON(join(ROOT, 'cards', cardId + '.json'), {
        id: cardId,
        type: card.type || '',
        title: card.title,
        description: card.description || '',
        context_level: 'project',
        due_date: null,
        tags: card.tags || [],
        created_at: created,
        updated_at: created,
        blocks: card.blocks || [],
      })

      const catIdv = catId[card.cat]
      const pos = (posInCat[catIdv] = (posInCat[catIdv] ?? -1) + 1)
      writeJSON(join(ROOT, 'pins', cardId, 'pins.json'), {
        card_id: cardId,
        // Real vaults set BOTH ids to the category UUID — mirror that.
        pins: [{ card_id: cardId, category_id: catIdv, project_id: catIdv, position: pos, pinned_at: created }],
      })

      if (card.agent) {
        const a = card.agent
        const { runs, lastAt, costSpent } = a.runs ? genRuns(cardId, a.runs) : { runs: [], lastAt: null, costSpent: 0 }
        // next run a little in the future so the fleet table reads "active".
        const nextAt = a.enabled ? iso(NOW + rint(20, 240) * 60 * 1000) : null
        const config = {
          enabled: a.enabled,
          goal: a.goal,
          schedule: a.schedule,
          allowed_tools: a.allowed_tools,
          status: a.enabled ? 'idle' : 'disabled',
          notify_on: ['failure'],
          notify_channel: a.notify_channel,
        }
        if (a.model) config.llm_model = a.model
        if (a.max_tokens_budget) config.max_tokens_budget = a.max_tokens_budget
        if (a.cost_budget_usd) config.cost_budget_usd = a.cost_budget_usd
        if (costSpent) config.cost_spent_usd = Math.round(costSpent * 1e6) / 1e6
        if (lastAt) config.last_run_at = lastAt
        if (nextAt) config.next_run_at = nextAt
        // Embedded runs — the app migrates them into its runs dir on first
        // open (see internal/repo/agent.go), which feeds the Agents tab.
        writeJSON(join(ROOT, 'cards', cardId + '.agent.json'), { card_id: cardId, config, runs })
        agentCount++; runCount += runs.length
      }
      if (card.featured) {
        writeJSON(join(ROOT, 'cards', cardId + '.comments.json'), {
          card_id: cardId,
          comments: [{
            id: 'cm-' + sid(), author: 'Harvey', created_at: stamp(), updated_at: stamp(),
            text: 'Genuinely lovely work — shipping it. 🚀',
          }],
        })
      }
      cardCount++; pinCount++
    })
  })
})

// global tag colour cache (keeps colours consistent across projects)
writeJSON(join(ROOT, 'tags.json'), Object.fromEntries(tagColor))

// empty dirs the app expects
for (const d of ['attachments', 'types']) mkdirSync(join(ROOT, d), { recursive: true })

console.log(`Built Claude's Board at ${ROOT}`)
console.log(`  ${cardCount} cards, ${pinCount} pins across ${b.streams.length} streams`)
console.log(`  ${agentCount} agents, ${runCount} agent runs`)
