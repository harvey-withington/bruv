import { describe, it, expect, beforeEach } from 'vitest'
import { OfflineEngine, OfflineUnavailableError, offlineCopyName, type RpcSend } from '@shared/offline/engine.svelte'
import { MemoryStorage } from '@shared/offline/storage'
import { countEffectRuns } from './testEffect.svelte'
import type { Block, Card, CardEdit } from '@shared/types'

class NetworkDown extends Error {}

const PIN = { brandName: 'Home', streamName: 'Blog', projectName: 'Writing', categoryName: 'Drafts', breadcrumb: 'Home / Blog / Writing / Drafts' }

/** A tiny in-memory BRUV server with revisions and guarded attachment saves. */
class FakeServer {
  cards = new Map<string, Card>()
  files = new Map<string, string>()
  down = false
  calls: string[] = []
  private hash(s: string) { return 'sha256:' + s.length + ':' + s }

  add(card: Card, files: Record<string, string> = {}) {
    this.cards.set(card.id, card)
    for (const [id, text] of Object.entries(files)) this.files.set(`${card.id}/${id}`, text)
  }
  private save(id: string, fn: (c: Card) => void): Card {
    const c = structuredClone(this.cards.get(id)!)
    fn(c)
    c.rev = (c.rev ?? 0) + 1
    this.cards.set(id, c)
    return structuredClone(c)
  }
  /** An edit made at home while the device was away. */
  homeEdit(id: string, fn: (c: Card) => void) { this.save(id, fn) }

  send: RpcSend = async (method, params) => {
    this.calls.push(method)
    if (this.down) throw new NetworkDown('fetch failed')
    const id = params[0] as string
    switch (method) {
      case 'GetCard': {
        const c = this.cards.get(id)
        if (!c) throw new Error(`card ${id} not found`)
        return structuredClone(c)
      }
      case 'CreateCardWithID': {
        const [cid, type, title] = params as [string, string, string]
        if (!this.cards.has(cid)) {
          // A typed card gets its type's fields at home.
          const blocks: Block[] = type ? [block('type-field', '')] : []
          this.cards.set(cid, { id: cid, title, description: '', type, tags: [], due_date: null, created_at: '', rev: 1, blocks, file_attachments: [] })
        }
        return structuredClone(this.cards.get(cid)!)
      }
      case 'ListCardTypes': return [{ id: 'blog', label: 'Blog Article' }]
      case 'GetCardPinBreadcrumbs': return [PIN]
      case 'UpdateCardDescription': return this.save(id, (c) => { c.description = params[1] as string })
      case 'UpdateCardTitle': return this.save(id, (c) => { c.title = params[1] as string })
      case 'CommitCardEdit': {
        const [, rev, edit] = params as [string, number, CardEdit]
        if ((this.cards.get(id)!.rev ?? 0) !== rev) return { conflict: true, card: structuredClone(this.cards.get(id)!) }
        return {
          conflict: false,
          card: this.save(id, (c) => {
            if (edit.title !== undefined) c.title = edit.title
            if (edit.description !== undefined) c.description = edit.description
            if (edit.tags) c.tags = edit.tags
            if (edit.blocks) c.blocks = edit.blocks
            if (edit.due_date !== undefined) c.due_date = edit.due_date || null
          }),
        }
      }
      case 'OpenCardAttachmentText': {
        const text = this.files.get(`${id}/${params[1]}`)
        if (text === undefined) throw new Error('attachment not found')
        return { content: text, stamp: { hash: this.hash(text), size: text.length } }
      }
      case 'SaveCardAttachmentText': {
        const [, att, content, expected] = params as [string, string, string, string]
        const cur = this.files.get(`${id}/${att}`)
        if (cur === undefined) throw new Error('attachment not found')
        if (expected && this.hash(cur) !== expected) return { diverged: true, stamp: { hash: this.hash(cur), size: cur.length } }
        this.files.set(`${id}/${att}`, content)
        this.save(id, () => {})
        return { diverged: false, stamp: { hash: this.hash(content), size: content.length } }
      }
      case 'AddCardAttachment': {
        const [, name, b64] = params as [string, string, string]
        const attId = `att-${name}`
        this.files.set(`${id}/${attId}`, new TextDecoder().decode(Uint8Array.from(atob(b64), (ch) => ch.charCodeAt(0))))
        return this.save(id, (c) => { c.file_attachments.push({ id: attId, name, path: '', mime: 'text/markdown', size: 0, added_at: '' }) })
      }
    }
    throw new Error(`unexpected ${method}`)
  }
}

const block = (id: string, value: string): Block => ({ id, type: 'text', label: id, key: '', value })
function blogCard(): Card {
  return {
    id: 'post', title: 'Plane post', description: 'intro\nbody\noutro', type: 'blog', tags: ['blog'], due_date: null,
    created_at: '2026-10-01T00:00:00Z', rev: 4, blocks: [block('notes', 'n')],
    file_attachments: [{ id: 'a1', name: 'post.md', path: '', mime: 'text/markdown', size: 5, added_at: '' }],
  }
}

let server: FakeServer
let engine: OfflineEngine
const call = (method: string, ...params: unknown[]) => engine.handle(method, params, server.send)

beforeEach(async () => {
  server = new FakeServer()
  server.add(blogCard(), { a1: '# Draft\nfirst para' })
  engine = new OfflineEngine({
    storage: new MemoryStorage(),
    reads: { ListCardTypes: 'board', GetCardPinBreadcrumbs: 'card' },
    isNetworkError: (e) => e instanceof NetworkDown,
    prefetch: async (_id, _card, fetch) => { await fetch('ListCardTypes', []) },
  })
  await engine.attach('https://ripped|vault', server.send)
  await engine.mark('post')
})

async function goOffline() {
  server.down = true
  engine.setWorking(true)
}
async function comeBack() {
  server.down = false
  engine.setWorking(false)
  await engine.sync()
}

describe('OfflineEngine', () => {
  it('downloads the card, its text attachments and warm reads', async () => {
    expect(engine.isMarked('post')).toBe(true)
    expect(engine.summary('post')?.status).toBe('synced')
    await goOffline()
    expect(((await call('GetCard', 'post')) as Card).title).toBe('Plane post')
    expect(await call('ListCardTypes')).toEqual([{ id: 'blog', label: 'Blog Article' }])
    expect(await call('GetCardPinBreadcrumbs', 'post')).toEqual([PIN])
    expect(engine.summary('post')?.places).toEqual([{ brand: 'Home', stream: 'Blog', project: 'Writing', category: 'Drafts' }])
    expect((await call('OpenCardAttachmentText', 'post', 'a1')) as { content: string }).toMatchObject({ content: '# Draft\nfirst para' })
  })

  it('a read made inside an effect does not re-run it (the card dialog stuck on loading)', async () => {
    const effect = countEffectRuns(() => { void call('GetCard', 'post') })
    for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0))
    expect(effect.runs()).toBe(1)
    effect.stop()
  })

  it('a live read of a marked card leaves its summary alone when nothing changed', async () => {
    const before = engine.cards
    await call('GetCard', 'post')
    await new Promise((r) => setTimeout(r, 0))
    expect(engine.cards).toBe(before)
  })

  it('refuses what it cannot do offline', async () => {
    await goOffline()
    await expect(call('MoveCardToCategory', 'post', 'x')).rejects.toBeInstanceOf(OfflineUnavailableError)
    await expect(call('GetCard', 'not-marked')).rejects.toBeInstanceOf(OfflineUnavailableError)
  })

  it('takes offline edits home when the card did not change meanwhile', async () => {
    await goOffline()
    await call('UpdateCardDescription', 'post', 'intro\nbody, written at 35,000 ft\noutro')
    await call('UpdateCardTitle', 'post', 'Plane post (v2)')
    expect(engine.summary('post')?.status).toBe('pending')
    await comeBack()
    const home = server.cards.get('post')!
    expect(home.description).toBe('intro\nbody, written at 35,000 ft\noutro')
    expect(home.title).toBe('Plane post (v2)')
    expect(engine.summary('post')?.status).toBe('synced')
    expect(server.calls.filter((c) => c === 'CommitCardEdit')).toHaveLength(1)
  })

  it('merges with edits made at home meanwhile', async () => {
    await goOffline()
    await call('UpdateCardDescription', 'post', 'intro, rewritten\nbody\noutro')
    server.homeEdit('post', (c) => {
      c.description = 'intro\nbody\noutro\nP.S. from home'
      c.blocks = [...c.blocks, block('agent', 'status: reviewed')]
    })
    await comeBack()
    const home = server.cards.get('post')!
    expect(home.description).toBe('intro, rewritten\nbody\noutro\nP.S. from home')
    expect(home.blocks.map((b) => b.id)).toEqual(['notes', 'agent'])
    expect(engine.summary('post')?.status).toBe('synced')
  })

  it('holds a real conflict for the user, then syncs their choice', async () => {
    await goOffline()
    await call('UpdateCardTitle', 'post', 'Offline title')
    server.homeEdit('post', (c) => { c.title = 'Home title' })
    await comeBack()
    expect(engine.summary('post')?.status).toBe('conflict')
    expect(server.cards.get('post')!.title).toBe('Home title')
    const { conflicts } = await engine.conflictsFor('post')
    expect(conflicts.map((c) => [c.id, c.mine, c.theirs])).toEqual([['title', 'Offline title', 'Home title']])

    await engine.resolve('post', 'title', 'mine')
    expect(server.cards.get('post')!.title).toBe('Offline title')
    expect(engine.summary('post')?.status).toBe('synced')
  })

  it('syncs an edited attachment, keeping both copies if home changed it too', async () => {
    await goOffline()
    const opened = (await call('OpenCardAttachmentText', 'post', 'a1')) as { stamp: { hash: string } }
    const saved = (await call('SaveCardAttachmentText', 'post', 'a1', '# Draft\nwritten offline', opened.stamp.hash)) as { diverged: boolean }
    expect(saved.diverged).toBe(false)
    server.files.set('post/a1', '# Draft\nedited at home')
    await comeBack()
    expect(server.files.get('post/a1')).toBe('# Draft\nedited at home')
    expect(server.files.get(`post/att-${offlineCopyName('post.md')}`)).toBe('# Draft\nwritten offline')
    expect(engine.summary('post')?.keptBoth).toEqual(['post.md'])
    expect(engine.summary('post')?.status).toBe('synced')
  })

  it('saves an edited attachment straight over an unchanged one', async () => {
    await goOffline()
    await call('SaveCardAttachmentText', 'post', 'a1', 'new text', '')
    await comeBack()
    expect(server.files.get('post/a1')).toBe('new text')
    expect(engine.summary('post')?.keptBoth).toEqual([])
  })

  it('answers from the device when a live call fails for want of a network', async () => {
    server.down = true // the surface hasn't noticed yet — no setWorking(true)
    expect(((await call('GetCard', 'post')) as Card).id).toBe('post')
    await call('UpdateCardDescription', 'post', 'typed as the wifi dropped')
    expect(engine.summary('post')?.status).toBe('pending')
    await expect(call('ListCards')).rejects.toBeInstanceOf(NetworkDown)
    server.down = false
    await engine.sync()
    expect(server.cards.get('post')!.description).toBe('typed as the wifi dropped')
  })

  it('keeps live responses fresh for marked cards', async () => {
    await call('UpdateCardDescription', 'post', 'edited online')
    await new Promise((r) => setTimeout(r, 0))
    await goOffline()
    expect(((await call('GetCard', 'post')) as Card).description).toBe('edited online')
  })

  it('a copy stored before pins were kept shows no place until refreshed', async () => {
    const storage = new MemoryStorage()
    const old = new OfflineEngine({ storage, reads: {}, isNetworkError: (e) => e instanceof NetworkDown })
    await old.attach('s|v', server.send)
    await old.mark('post')
    const rec = (await storage.listCards('s|v'))[0]
    delete rec.places
    await storage.putCard(rec)
    await old.attach('s|v', server.send) // reload
    expect(old.summary('post')?.places).toBeNull()
    await old.refreshAll()
    expect(old.summary('post')?.places?.[0]?.project).toBe('Writing')

    // Places kept before the full path was are refetched too.
    const rec2 = (await storage.listCards('s|v'))[0]
    rec2.places = [{ project: 'Writing', category: 'Drafts' }]
    await storage.putCard(rec2)
    await old.attach('s|v', server.send)
    await old.refreshAll()
    expect(old.summary('post')?.places).toEqual([{ brand: 'Home', stream: 'Blog', project: 'Writing', category: 'Drafts' }])
  })

  it('keeps travel documents (non-text attachments) and opens them offline', async () => {
    let n = 0
    const urls: string[] = []
    const createURL = URL.createObjectURL
    const revokeURL = URL.revokeObjectURL
    URL.createObjectURL = () => `blob:device/${++n}`
    URL.revokeObjectURL = (u: string) => { urls.push(u) }
    try {
      const card = server.cards.get('post')!
      card.file_attachments.push(
        { id: 'pass', name: 'boarding-pass.pdf', path: '', mime: 'application/pdf', size: 3, added_at: '' },
        { id: 'huge', name: 'video.mp4', path: '', mime: 'video/mp4', size: 400 * 1024 * 1024, added_at: '' },
      )
      const fetched: string[] = []
      const storage = new MemoryStorage()
      const e = new OfflineEngine({
        storage, reads: {}, isNetworkError: (err) => err instanceof NetworkDown,
        fetchFile: async (_c, att) => { fetched.push(att); return new Blob(['pdf'], { type: 'application/pdf' }) },
      })
      await e.attach('s|v', server.send)
      await e.mark('post')
      expect(fetched).toEqual(['pass']) // text goes via its own path, over-cap stays online-only
      expect(e.summary('post')!.bytes).toBeGreaterThan(3)

      server.down = true
      e.setWorking(true)
      const url = await e.handle('SignAttachmentURL', ['post', 'pass'], server.send)
      expect(url).toBe('blob:device/1')
      expect(await e.handle('SignAttachmentURL', ['post', 'pass'], server.send)).toBe('blob:device/1') // reused
      await expect(e.handle('SignAttachmentURL', ['post', 'huge'], server.send)).rejects.toBeInstanceOf(OfflineUnavailableError)

      // Unchanged files aren't fetched again; removed ones are dropped.
      server.down = false
      e.setWorking(false)
      await e.download('post')
      expect(fetched).toEqual(['pass'])
      card.file_attachments = card.file_attachments.filter((a) => a.id !== 'pass')
      await e.download('post')
      expect(urls).toContain('blob:device/1')
      e.setWorking(true)
      await expect(e.handle('SignAttachmentURL', ['post', 'pass'], server.send)).rejects.toBeInstanceOf(OfflineUnavailableError)
    } finally {
      URL.createObjectURL = createURL
      URL.revokeObjectURL = revokeURL
    }
  })

  it('creates a card offline (Inbox) and makes it at home under the same id', async () => {
    await goOffline()
    const made = await engine.createOffline('Song idea')
    expect(engine.summary(made.id)).toMatchObject({ created: true, status: 'pending', places: [] })
    expect(((await call('GetCard', made.id)) as Card).title).toBe('Song idea')
    expect(await call('GetCardPinBreadcrumbs', made.id)).toBeNull() // nothing at home yet
    await call('UpdateCardDescription', made.id, 'verse in D, chorus lifts to G')
    expect(server.cards.has(made.id)).toBe(false)

    await comeBack()
    const home = server.cards.get(made.id)!
    expect(home.title).toBe('Song idea')
    expect(home.description).toBe('verse in D, chorus lifts to G')
    expect(engine.summary(made.id)).toMatchObject({ created: false, status: 'synced' })
    expect(server.calls.filter((c) => c === 'CreateCardWithID')).toHaveLength(1)
  })

  it('a card made offline stays local even while online until it syncs', async () => {
    const made = await engine.createOffline('Typed before the sync ran')
    // Online, but not created at home yet: answered here, not a 404 from the server.
    expect(((await call('GetCard', made.id)) as Card).id).toBe(made.id)
    await engine.sync()
    expect(server.cards.get(made.id)?.title).toBe('Typed before the sync ran')
  })

  it('attachments added offline show at once, open offline and upload on sync', async () => {
    URL.createObjectURL = () => 'blob:device/added'
    await goOffline()
    const made = await engine.createOffline('Visa run')
    const b64 = (s: string) => btoa(s)
    let card = (await call('AddCardAttachment', made.id, 'visa.pdf', b64('%PDF visa'))) as Card
    card = (await call('AddCardAttachment', made.id, 'notes.md', b64('# Notes'))) as Card
    expect(card.file_attachments.map((a) => a.name)).toEqual(['visa.pdf', 'notes.md'])
    const [pdf, md] = card.file_attachments
    expect(await call('SignAttachmentURL', made.id, pdf.id)).toBe('blob:device/added')
    const opened = (await call('OpenCardAttachmentText', made.id, md.id)) as { content: string; stamp: { hash: string } }
    expect(opened.content).toBe('# Notes')
    await call('SaveCardAttachmentText', made.id, md.id, '# Notes\nfrom the plane', opened.stamp.hash)
    // Taking one back before syncing just forgets it.
    const tmp = (await call('AddCardAttachment', made.id, 'oops.png', b64('png'))) as Card
    await call('RemoveCardAttachment', made.id, tmp.file_attachments[2].id)

    await comeBack()
    expect(server.files.get(`${made.id}/att-visa.pdf`)).toBe('%PDF visa')
    expect(server.files.get(`${made.id}/att-notes.md`)).toBe('# Notes\nfrom the plane')
    expect(server.cards.get(made.id)!.file_attachments.map((a) => a.name)).toEqual(['visa.pdf', 'notes.md'])
    expect(engine.summary(made.id)?.status).toBe('synced')
  })

  it('unmarking forgets the card', async () => {
    await engine.unmark('post')
    expect(engine.isMarked('post')).toBe(false)
    await goOffline()
    await expect(call('ListCardTypes')).rejects.toBeInstanceOf(OfflineUnavailableError)
  })

  it('a stale offline sync never overwrites a newer home edit of the same field', async () => {
    await goOffline()
    await call('UpdateCardDescription', 'post', 'offline version')
    server.homeEdit('post', (c) => { c.description = 'home version' })
    await comeBack()
    expect(server.cards.get('post')!.description).toBe('home version')
    expect(engine.summary('post')?.status).toBe('conflict')
    await engine.resolve('post', 'description', 'both')
    expect(server.cards.get('post')!.description).toBe('home version\n\n---\n\noffline version')
  })
})
