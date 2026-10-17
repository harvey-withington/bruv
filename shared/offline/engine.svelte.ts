// Offline cards: the engine both surfaces share.
//
// A surface routes every RPC through `handle(method, params, send)`.
// Online, calls go to the server as before and the engine keeps the
// device's copy of each card marked for offline fresh from what comes
// back. Offline, or when a call fails for want of a network, the engine
// answers from that copy instead: reads from the cache, content edits
// into a local working copy. Back online, `sync()` takes each working
// copy home — committed against the revision it was based on, merged
// three-way when the card moved meanwhile, and handed to the user only
// where both sides changed the same thing.
//
// Design: BRUV card "Offline cards" (08a5bb15) and plan/2026-10-17
// offline cards.md.

import type { Card, CardCommitResult, CardEdit, CardPin, WorkspaceFileContent, WorkspaceFileStamp, WorkspaceSaveResult } from '../types'
import { isEditableTextAttachment } from '../attachmentText'
import { applyResolutions, cardEditBetween, isEmptyEdit, mergeCard, type ConflictChoice, type OfflineConflict } from './merge'
import { cardKey, responseKey, type OfflineAttachment, type OfflineCardRecord, type OfflinePlace, type OfflineStorage } from './storage'

export type RpcSend = (method: string, params: unknown[]) => Promise<unknown>

/** Thrown for a call the device can't answer while offline. */
export class OfflineUnavailableError extends Error {
  constructor(readonly method: string) {
    super(`${method} isn't available offline`)
    this.name = 'OfflineUnavailableError'
  }
}

export type OfflineCardStatus = 'synced' | 'pending' | 'conflict' | 'error'

/** What the UI shows for one offline card. */
export type OfflineCardSummary = {
  cardId: string
  title: string
  status: OfflineCardStatus
  fetchedAt: number
  syncedAt: number | null
  conflicts: number
  syncError: string | null
  keptBoth: string[]
  /** Where it's pinned — its projects aren't on the device, so this is
   *  the only way to tell offline. Empty = unfiled (Inbox); null = not
   *  known yet (a copy stored before pins were kept, until it refreshes). */
  places: OfflinePlace[] | null
  /** Bytes of text held for it (card + attachments), for the size readout. */
  bytes: number
}

export type SyncReport = { synced: number; conflicts: number; failed: number }

export interface OfflineEngineOptions {
  storage: OfflineStorage
  /**
   * Reads worth answering offline. 'card': the first param is a card id,
   * cached for marked cards only. 'board': vault-level (card types, tag
   * colours, a project's tags), cached while any card is marked.
   */
  reads: Record<string, 'card' | 'board'>
  /** Extra reads to warm when a card is downloaded (breadcrumbs, project
   *  tags…). Each call through `fetch` is cached like a live read. */
  prefetch?: (cardId: string, card: Card, fetch: RpcSend) => Promise<void>
  /** True for "the server can't be reached", as opposed to a server error. */
  isNetworkError: (e: unknown) => boolean
  /** A sync took something home or found conflicts (for a toast). */
  onReport?: (report: SyncReport) => void
  now?: () => number
}

// Content edits the engine can take offline. The type, placement,
// attachments list and everything else need the server.
const CARD_WRITES = new Set(['UpdateCardTitle', 'UpdateCardDescription', 'UpdateCardBlocks', 'UpdateCardTags', 'UpdateCardDueDate'])
const TEXT_ATTACHMENT_CALLS = new Set(['OpenCardAttachmentText', 'StatCardAttachmentText', 'SaveCardAttachmentText'])
// Calls whose result is the card as saved — each refreshes a clean copy.
const CARD_RESULTS = new Set(['GetCard', ...CARD_WRITES, 'UpdateCardType', 'RefreshTypeBlocks', 'AddCardAttachment', 'RemoveCardAttachment'])

/** How long a downloaded copy counts as fresh before a refresh re-fetches it. */
const REFRESH_AFTER_MS = 2 * 60 * 1000

export class OfflineEngine {
  /** The server + vault the cache is for; null until attached. */
  scope = $state<string | null>(null)
  cards = $state<OfflineCardSummary[]>([])
  /** Working offline: every call is answered from the device. */
  working = $state(false)
  syncing = $state(false)

  private send: RpcSend | null = null
  private locks = new Map<string, Promise<unknown>>()
  private syncRun: Promise<SyncReport> | null = null
  private syncAgain = false
  private readonly now: () => number

  constructor(private readonly opts: OfflineEngineOptions) {
    this.now = opts.now ?? Date.now
  }

  /** Point the engine at a server + vault and the transport to reach it. */
  async attach(scope: string, send: RpcSend): Promise<void> {
    this.scope = scope
    this.send = send
    try {
      await this.refreshSummaries()
    } catch (e) {
      // Storage unusable here: stay a pass-through rather than half-attached.
      this.scope = null
      this.send = null
      throw e
    }
  }

  isMarked(cardId: string): boolean {
    return this.cards.some((c) => c.cardId === cardId)
  }

  summary(cardId: string): OfflineCardSummary | undefined {
    return this.cards.find((c) => c.cardId === cardId)
  }

  get pendingCount(): number {
    return this.cards.filter((c) => c.status !== 'synced').length
  }

  /** Enter or leave working offline. Leaving syncs and refreshes. */
  setWorking(working: boolean): void {
    if (this.working === working) return
    this.working = working
    if (!working) void this.afterReconnect()
  }

  /** Back online: take edits home, then freshen the clean copies. */
  async afterReconnect(): Promise<void> {
    if (this.working || !this.scope) return
    await this.sync()
    await this.refreshAll()
  }

  // --- marking ---

  /** Make a card available offline and download it now. */
  async mark(cardId: string): Promise<void> {
    const { scope } = this.requireAttached()
    const key = cardKey(scope, cardId)
    await this.withCard(key, async () => {
      if (await this.opts.storage.getCard(key)) return
      const card = (await this.transport()('GetCard', [cardId])) as Card
      await this.opts.storage.putCard(newRecord(scope, card, this.now()))
    })
    await this.refreshSummaries() // marked from here on, so its warm reads cache
    await this.download(cardId)
  }

  /** Stop keeping a card offline. Unsynced edits are discarded — callers
   *  confirm first when the card isn't synced. */
  async unmark(cardId: string): Promise<void> {
    const { scope } = this.requireAttached()
    const key = cardKey(scope, cardId)
    await this.withCard(key, async () => {
      await this.opts.storage.deleteCard(key)
      await this.opts.storage.deleteResponses(scope, cardId)
    })
    if ((await this.opts.storage.listCards(scope)).length === 0) await this.opts.storage.deleteResponses(scope, undefined)
    await this.refreshSummaries()
  }

  /** Re-download a card: its server copy, text attachments and warm reads.
   *  A card with unsynced edits keeps its working copy (sync owns that). */
  async download(cardId: string): Promise<void> {
    const { scope } = this.requireAttached()
    const send = this.transport()
    const key = cardKey(scope, cardId)
    const card = (await send('GetCard', [cardId])) as Card
    const texts: Record<string, WorkspaceFileContent> = {}
    for (const att of card.file_attachments ?? []) {
      if (!isEditableTextAttachment(att)) continue
      try {
        texts[att.id] = (await send('OpenCardAttachmentText', [cardId, att.id])) as WorkspaceFileContent
      } catch (e) {
        if (this.opts.isNetworkError(e)) throw e
        // Too big or not really text — it just won't open offline.
      }
    }
    await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      if (!rec) return // unmarked meanwhile
      if (isClean(rec)) rec.base = card
      rec.attachments = mergeAttachments(rec.attachments, card, texts)
      rec.fetchedAt = this.now()
      await this.opts.storage.putCard(rec)
    })
    const fetch: RpcSend = async (method, params) => {
      const value = await send(method, params)
      await this.cacheRead(scope, method, params, value)
      return value
    }
    // Every card-keyed read, plus the pins (always — the offline list
    // shows where each card lives).
    const cardReads = new Set(Object.entries(this.opts.reads).filter(([m, kind]) => kind === 'card' && m !== 'GetCard').map(([m]) => m))
    cardReads.add('GetCardPinBreadcrumbs')
    for (const method of cardReads) await fetch(method, [cardId]).catch(() => undefined)
    await this.opts.prefetch?.(cardId, card, fetch).catch(() => undefined)
    await this.refreshSummaries()
  }

  /** Re-download every clean card that isn't fresh, and fill in the pins
   *  of any copy that doesn't know them in full yet (even an edited one —
   *  pins aren't part of its edits). */
  async refreshAll(): Promise<void> {
    if (!this.scope || this.working) return
    const scope = this.scope
    for (const rec of await this.opts.storage.listCards(scope)) {
      try {
        if (isClean(rec) && this.now() - rec.fetchedAt >= REFRESH_AFTER_MS) await this.download(rec.cardId)
        else if (!rec.places || rec.places.some((p) => p.brand === undefined)) await this.notePlaces(scope, rec.cardId, ((await this.transport()('GetCardPinBreadcrumbs', [rec.cardId])) as CardPin[] | null) ?? [])
      } catch (e) {
        if (this.opts.isNetworkError(e)) return
        // A card deleted at home: leave the copy; the user can unmark it.
      }
    }
  }

  // --- the interceptor ---

  /** Route one RPC: to the server, or to the device's copy. */
  async handle(method: string, params: unknown[], send: RpcSend): Promise<unknown> {
    if (!this.scope) return send(method, params)
    const cardId = typeof params[0] === 'string' ? params[0] : ''
    if (this.working) return this.answerOffline(method, params)

    // A card with unsynced edits keeps taking them locally until sync
    // has taken it home, so edits land in order.
    if (cardId && this.isMarked(cardId) && (CARD_WRITES.has(method) || TEXT_ATTACHMENT_CALLS.has(method) || method === 'GetCard')) {
      const status = this.summary(cardId)?.status
      if (status === 'pending' || status === 'conflict') {
        const value = await this.answerOffline(method, params)
        if (CARD_WRITES.has(method) || method === 'SaveCardAttachmentText') void this.sync()
        return value
      }
      if (method === 'SaveCardAttachmentText') params = await this.serverHashFor(params)
    }

    try {
      const value = await send(method, params)
      void this.record(method, params, value).catch(() => undefined)
      return value
    } catch (e) {
      if (this.opts.isNetworkError(e) && (await this.canAnswer(method, params))) return this.answerOffline(method, params)
      throw e
    }
  }

  private async canAnswer(method: string, params: unknown[]): Promise<boolean> {
    const cardId = typeof params[0] === 'string' ? params[0] : ''
    if (cardId && this.isMarked(cardId) && (CARD_WRITES.has(method) || TEXT_ATTACHMENT_CALLS.has(method) || method === 'GetCard')) return true
    if (!this.opts.reads[method] || !this.scope) return false
    return !!(await this.opts.storage.getResponse(responseKey(this.scope, method, params)))
  }

  private async answerOffline(method: string, params: unknown[]): Promise<unknown> {
    const scope = this.scope!
    const cardId = typeof params[0] === 'string' ? params[0] : ''
    if (cardId && this.isMarked(cardId)) {
      if (method === 'GetCard') {
        const rec = await this.opts.storage.getCard(cardKey(scope, cardId))
        if (rec) return rec.local ?? rec.base
      }
      if (CARD_WRITES.has(method)) return this.editLocally(cardId, method, params)
      if (TEXT_ATTACHMENT_CALLS.has(method)) return this.attachmentLocally(cardId, method, params)
    }
    if (this.opts.reads[method]) {
      const hit = await this.opts.storage.getResponse(responseKey(scope, method, params))
      if (hit) return hit.value
    }
    throw new OfflineUnavailableError(method)
  }

  /** Keep the cache fresh from a live response. */
  private async record(method: string, params: unknown[], value: unknown): Promise<void> {
    const scope = this.scope
    if (!scope || this.cards.length === 0) return
    const cardId = typeof params[0] === 'string' ? params[0] : ''
    if (cardId && this.isMarked(cardId)) {
      const card = method === 'CommitCardEdit' ? (value as CardCommitResult)?.card : CARD_RESULTS.has(method) ? (value as Card) : null
      if (card && typeof card === 'object' && card.id === cardId) await this.refreshBase(scope, card)
      if (method === 'OpenCardAttachmentText' || method === 'SaveCardAttachmentText') await this.refreshAttachment(scope, cardId, method, params, value)
    }
    await this.cacheRead(scope, method, params, value)
  }

  private async cacheRead(scope: string, method: string, params: unknown[], value: unknown): Promise<void> {
    if (method === 'GetCardPinBreadcrumbs' && typeof params[0] === 'string' && this.isMarked(params[0])) {
      await this.notePlaces(scope, params[0], (value as CardPin[] | null) ?? [])
    }
    const kind = this.opts.reads[method]
    if (!kind || method === 'GetCard' || value === undefined) return
    const cardId = kind === 'card' && typeof params[0] === 'string' ? params[0] : undefined
    if (kind === 'card' && !(cardId && this.isMarked(cardId))) return
    await this.opts.storage.putResponse({ key: responseKey(scope, method, params), scope, cardId, value, at: this.now() })
  }

  private async notePlaces(scope: string, cardId: string, pins: CardPin[]): Promise<void> {
    const places = pins.map((p) => ({ brand: p.brandName, stream: p.streamName, project: p.projectName, category: p.categoryName }))
    const key = cardKey(scope, cardId)
    await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      if (!rec) return
      rec.places = places
      await this.opts.storage.putCard(rec)
    })
    this.patchSummary(cardId, { places })
  }

  private async refreshBase(scope: string, card: Card): Promise<void> {
    const key = cardKey(scope, card.id)
    await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      if (!rec || !isClean(rec)) return
      rec.base = card
      rec.attachments = mergeAttachments(rec.attachments, card, {})
      rec.fetchedAt = this.now()
      await this.opts.storage.putCard(rec)
    })
    this.patchSummary(card.id, { title: card.title })
  }

  private async refreshAttachment(scope: string, cardId: string, method: string, params: unknown[], value: unknown): Promise<void> {
    const attId = String(params[1])
    let content: string, hash: string | undefined
    if (method === 'OpenCardAttachmentText') {
      const open = value as WorkspaceFileContent
      content = open.content; hash = open.stamp?.hash
    } else {
      const res = value as WorkspaceSaveResult
      if (res.diverged) return
      content = String(params[2]); hash = res.stamp?.hash
    }
    if (!hash) return
    const key = cardKey(scope, cardId)
    await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      const att = rec?.attachments[attId]
      if (!rec || !att || att.local !== null) return
      att.base = content
      att.baseHash = hash!
      await this.opts.storage.putCard(rec)
    })
  }

  /** The editor holds a device stamp after an offline edit; the server
   *  only knows its own. Swap in the stamp of the copy those edits were
   *  made on, so a change at home still shows as divergence. */
  private async serverHashFor(params: unknown[]): Promise<unknown[]> {
    const expected = String(params[3] ?? '')
    if (!expected.startsWith(LOCAL_HASH_PREFIX)) return params
    const rec = await this.opts.storage.getCard(cardKey(this.scope!, String(params[0])))
    const att = rec?.attachments[String(params[1])]
    return att ? [params[0], params[1], params[2], att.baseHash] : params
  }

  // --- local edits ---

  private async editLocally(cardId: string, method: string, params: unknown[]): Promise<Card> {
    const key = cardKey(this.scope!, cardId)
    const card = await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      if (!rec) throw new OfflineUnavailableError(method)
      const work: Card = structuredCloneCard(rec.local ?? rec.base)
      const arg = params[1]
      switch (method) {
        case 'UpdateCardTitle': work.title = String(arg ?? ''); break
        case 'UpdateCardDescription': work.description = String(arg ?? ''); break
        case 'UpdateCardBlocks': work.blocks = (arg as Card['blocks']) ?? []; break
        case 'UpdateCardTags': work.tags = (arg as string[]) ?? []; break
        case 'UpdateCardDueDate': work.due_date = String(arg ?? '') || null; break
      }
      work.updated_at = new Date(this.now()).toISOString()
      rec.local = work
      await this.opts.storage.putCard(rec)
      return work
    })
    this.patchSummary(cardId, { title: card.title, status: this.summary(cardId)?.status === 'conflict' ? 'conflict' : 'pending' })
    return card
  }

  private async attachmentLocally(cardId: string, method: string, params: unknown[]): Promise<unknown> {
    const key = cardKey(this.scope!, cardId)
    const attId = String(params[1])
    const result = await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      const att = rec?.attachments[attId]
      if (!rec || !att) throw new OfflineUnavailableError(method)
      if (method === 'OpenCardAttachmentText') return { content: att.local ?? att.base, stamp: stampOf(att) } satisfies WorkspaceFileContent
      if (method === 'StatCardAttachmentText') return stampOf(att) satisfies WorkspaceFileStamp
      const content = String(params[2] ?? '')
      att.local = content === att.base ? null : content
      await this.opts.storage.putCard(rec)
      return { diverged: false, stamp: stampOf(att) } satisfies WorkspaceSaveResult
    })
    if (method === 'SaveCardAttachmentText') await this.refreshSummaries()
    return result
  }

  // --- sync ---

  /** Take every card's offline edits home. Runs one at a time; a call
   *  during a run queues one more run. */
  sync(): Promise<SyncReport> {
    if (this.syncRun) {
      this.syncAgain = true
      return this.syncRun
    }
    this.syncRun = (async () => {
      this.syncing = true
      let report: SyncReport = { synced: 0, conflicts: 0, failed: 0 }
      try {
        do {
          this.syncAgain = false
          report = await this.syncOnce()
        } while (this.syncAgain && !this.working)
      } finally {
        this.syncing = false
        this.syncRun = null
        await this.refreshSummaries()
      }
      if (report.synced || report.conflicts) this.opts.onReport?.(report)
      return report
    })()
    return this.syncRun
  }

  private async syncOnce(): Promise<SyncReport> {
    const report: SyncReport = { synced: 0, conflicts: 0, failed: 0 }
    if (!this.scope || this.working || !this.send) return report
    for (const rec of await this.opts.storage.listCards(this.scope)) {
      if (isClean(rec) && !rec.syncError) continue
      if (rec.conflicts.length && !allResolved(rec)) { report.conflicts++; continue }
      try {
        const outcome = await this.syncCard(rec.key)
        report[outcome]++
      } catch (e) {
        if (this.opts.isNetworkError(e)) break // still offline — try again on reconnect
        report.failed++
        await this.withCard(rec.key, async () => {
          const cur = await this.opts.storage.getCard(rec.key)
          if (!cur) return
          cur.syncError = e instanceof Error ? e.message : String(e)
          await this.opts.storage.putCard(cur)
        })
      }
    }
    return report
  }

  private async syncCard(key: string): Promise<'synced' | 'conflicts'> {
    const send = this.transport()
    const start = await this.opts.storage.getCard(key)
    if (!start) return 'synced'
    const cardId = start.cardId

    // Resolved conflicts become an ordinary edit on top of the server copy
    // they were found against.
    if (start.conflicts.length && allResolved(start) && start.conflictServer && start.local) {
      const { card: merged } = mergeCard(start.base, start.local, start.conflictServer)
      const resolved = applyResolutions(merged, start.local, start.conflicts, start.resolutions)
      await this.withCard(key, async () => {
        const cur = await this.opts.storage.getCard(key)
        if (!cur) return
        cur.base = start.conflictServer!
        cur.local = resolved
        cur.conflicts = []
        cur.resolutions = {}
        cur.conflictServer = null
        await this.opts.storage.putCard(cur)
      })
    }

    const rec = (await this.opts.storage.getCard(key))!
    const sentLocal = rec.local
    let server = (await send('GetCard', [cardId])) as Card
    let conflicts: OfflineConflict[] = []

    if (sentLocal) {
      for (let attempt = 0; ; attempt++) {
        let target = sentLocal
        if ((server.rev ?? 0) !== (rec.base.rev ?? 0)) {
          const merged = mergeCard(rec.base, sentLocal, server)
          if (merged.conflicts.length) { conflicts = merged.conflicts; break }
          target = merged.card
        }
        const edit: CardEdit = cardEditBetween(server, target)
        if (isEmptyEdit(edit)) break
        const res = (await send('CommitCardEdit', [cardId, server.rev ?? 0, edit])) as CardCommitResult
        if (!res.conflict) { server = res.card; break }
        if (attempt >= 3) throw new Error('the card kept changing at home while syncing; try again')
        server = res.card
      }
    }

    // Text attachments: a guarded save on the hash they were downloaded
    // at; if the file changed at home too, keep both copies.
    const sentAttachments = new Map<string, string>()
    const keptBoth: string[] = []
    for (const att of Object.values(rec.attachments)) {
      if (att.local === null) continue
      sentAttachments.set(att.id, att.local)
      let saved: WorkspaceSaveResult | null = null
      try {
        saved = (await send('SaveCardAttachmentText', [cardId, att.id, att.local, att.baseHash])) as WorkspaceSaveResult
      } catch (e) {
        if (this.opts.isNetworkError(e)) throw e
        // Removed at home: the offline copy comes back as a new attachment.
      }
      if (!saved || saved.diverged) {
        await send('AddCardAttachment', [cardId, saved ? offlineCopyName(att.name) : att.name, utf8Base64(att.local)])
        keptBoth.push(att.name)
      }
    }

    const fresh = (await send('GetCard', [cardId])) as Card
    const texts: Record<string, WorkspaceFileContent> = {}
    for (const id of sentAttachments.keys()) {
      try {
        texts[id] = (await send('OpenCardAttachmentText', [cardId, id])) as WorkspaceFileContent
      } catch { /* gone at home — dropped from the copy below */ }
    }

    await this.withCard(key, async () => {
      const cur = await this.opts.storage.getCard(key)
      if (!cur) return
      if (conflicts.length) {
        cur.conflicts = conflicts
        cur.conflictServer = server
        cur.resolutions = {}
      } else {
        // Edits made while this sync ran are kept, rebased onto the result.
        cur.local = cur.local && sentLocal && !sameCard(cur.local, sentLocal)
          ? mergeCard(sentLocal, cur.local, fresh).card
          : null
        cur.base = fresh
      }
      for (const [id, sent] of sentAttachments) {
        const att = cur.attachments[id]
        if (!att) continue
        const open = texts[id]
        if (!open) { delete cur.attachments[id]; continue }
        att.base = open.content
        att.baseHash = open.stamp?.hash ?? ''
        if (att.local === sent) att.local = null
      }
      cur.attachments = mergeAttachments(cur.attachments, fresh, {})
      cur.keptBoth = keptBoth
      cur.syncError = null
      cur.syncedAt = this.now()
      cur.fetchedAt = this.now()
      await this.opts.storage.putCard(cur)
    })
    return conflicts.length ? 'conflicts' : 'synced'
  }

  // --- conflicts ---

  async conflictsFor(cardId: string): Promise<{ conflicts: OfflineConflict[]; resolutions: Record<string, ConflictChoice> }> {
    const rec = this.scope ? await this.opts.storage.getCard(cardKey(this.scope, cardId)) : undefined
    return { conflicts: rec?.conflicts ?? [], resolutions: rec?.resolutions ?? {} }
  }

  /** Record a choice for one conflict; once every conflict has one, the
   *  card syncs with them applied. */
  async resolve(cardId: string, conflictId: string, choice: ConflictChoice): Promise<void> {
    const key = cardKey(this.requireAttached().scope, cardId)
    let ready = false
    await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      if (!rec || !rec.conflicts.some((c) => c.id === conflictId)) return
      rec.resolutions[conflictId] = choice
      ready = allResolved(rec)
      await this.opts.storage.putCard(rec)
    })
    if (ready && !this.working) await this.sync()
    else await this.refreshSummaries()
  }

  /** Clear the "kept both copies" notice once the user has seen it. */
  async dismissKeptBoth(cardId: string): Promise<void> {
    const key = cardKey(this.requireAttached().scope, cardId)
    await this.withCard(key, async () => {
      const rec = await this.opts.storage.getCard(key)
      if (!rec) return
      rec.keptBoth = []
      await this.opts.storage.putCard(rec)
    })
    await this.refreshSummaries()
  }

  // --- plumbing ---

  private requireAttached(): { scope: string } {
    if (!this.scope) throw new Error('offline cards are not set up for this vault')
    return { scope: this.scope }
  }

  private transport(): RpcSend {
    if (!this.send || this.working) throw new OfflineUnavailableError('sync')
    return this.send
  }

  /** Serialize work on one card: read-modify-write of its record never interleaves. */
  private withCard<T>(key: string, fn: () => Promise<T>): Promise<T> {
    const prev = this.locks.get(key) ?? Promise.resolve()
    const run = prev.then(fn, fn)
    const tail = run.catch(() => undefined)
    this.locks.set(key, tail)
    void tail.then(() => { if (this.locks.get(key) === tail) this.locks.delete(key) })
    return run
  }

  private async refreshSummaries(): Promise<void> {
    if (!this.scope) { this.cards = []; return }
    const recs = await this.opts.storage.listCards(this.scope)
    this.cards = recs.map(summarize).sort((a, b) => a.title.localeCompare(b.title))
  }

  private patchSummary(cardId: string, patch: Partial<OfflineCardSummary>): void {
    this.cards = this.cards.map((c) => (c.cardId === cardId ? { ...c, ...patch } : c))
  }
}

// --- helpers ---

const LOCAL_HASH_PREFIX = 'device:'

function newRecord(scope: string, card: Card, now: number): OfflineCardRecord {
  return {
    key: cardKey(scope, card.id), scope, cardId: card.id,
    base: card, local: null, attachments: {},
    conflicts: [], conflictServer: null, resolutions: {},
    markedAt: now, fetchedAt: now, syncedAt: null, syncError: null, keptBoth: [],
  }
}

function isClean(rec: OfflineCardRecord): boolean {
  return rec.local === null && rec.conflicts.length === 0 && Object.values(rec.attachments).every((a) => a.local === null)
}

function allResolved(rec: OfflineCardRecord): boolean {
  return rec.conflicts.every((c) => rec.resolutions[c.id])
}

function summarize(rec: OfflineCardRecord): OfflineCardSummary {
  const card = rec.local ?? rec.base
  const status: OfflineCardStatus = rec.conflicts.length && !allResolved(rec)
    ? 'conflict'
    : !isClean(rec) ? 'pending' : rec.syncError ? 'error' : 'synced'
  let bytes = JSON.stringify(card).length
  for (const a of Object.values(rec.attachments)) bytes += a.base.length + (a.local?.length ?? 0)
  return {
    cardId: rec.cardId, title: card.title, status,
    fetchedAt: rec.fetchedAt, syncedAt: rec.syncedAt,
    conflicts: rec.conflicts.length, syncError: rec.syncError, keptBoth: rec.keptBoth,
    places: rec.places ?? null, bytes,
  }
}

/** The card's current text attachments: fresh content where downloaded,
 *  edited copies kept as they are, entries the card no longer has dropped
 *  (unless they hold unsynced edits — sync turns those into a new
 *  attachment). */
function mergeAttachments(
  current: Record<string, OfflineAttachment>, card: Card, texts: Record<string, WorkspaceFileContent>,
): Record<string, OfflineAttachment> {
  const out: Record<string, OfflineAttachment> = {}
  for (const att of card.file_attachments ?? []) {
    if (!isEditableTextAttachment(att)) continue
    const prev = current[att.id]
    const text = texts[att.id]
    // An edited copy keeps the base its edits were made on — the guarded
    // save at sync time depends on that hash.
    if (text && !(prev && prev.local !== null)) out[att.id] = { id: att.id, name: att.name, base: text.content, baseHash: text.stamp?.hash ?? '', local: null }
    else if (prev) out[att.id] = { ...prev, name: att.name }
  }
  for (const [id, prev] of Object.entries(current)) if (!out[id] && prev.local !== null) out[id] = prev
  return out
}

function stampOf(att: OfflineAttachment): WorkspaceFileStamp {
  if (att.local === null) return { hash: att.baseHash, size: utf8Length(att.base) }
  return { hash: LOCAL_HASH_PREFIX + fnv1a(att.local), size: utf8Length(att.local) }
}

function fnv1a(s: string): string {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  return (h >>> 0).toString(16).padStart(8, '0') + s.length.toString(16)
}

function utf8Length(s: string): number {
  return new TextEncoder().encode(s).length
}

function utf8Base64(s: string): string {
  let bin = ''
  for (const byte of new TextEncoder().encode(s)) bin += String.fromCharCode(byte)
  return btoa(bin)
}

/** "post.md" → "post (offline).md" */
export function offlineCopyName(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot > 0 ? `${name.slice(0, dot)} (offline)${name.slice(dot)}` : `${name} (offline)`
}

function structuredCloneCard(card: Card): Card {
  return JSON.parse(JSON.stringify(card)) as Card
}

function sameCard(a: Card, b: Card): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}
