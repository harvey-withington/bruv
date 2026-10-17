// Where offline cards live on the device.
//
// IndexedDB in the browser engine both surfaces run in (WebView2 on the
// desktop, the PWA on mobile); an in-memory store for tests. Records are
// keyed by scope (server + vault), so two vaults or two servers never
// share a cache.

import type { Card } from '../types'
import type { ConflictChoice, OfflineConflict } from './merge'

/** A text attachment's offline copy. */
export type OfflineAttachment = {
  id: string
  name: string
  /** Content as downloaded, and the server's stamp hash for it. */
  base: string
  baseHash: string
  /** Edited offline and not yet synced; null when clean. */
  local: string | null
}

/** Where the card is pinned, for showing offline. */
export type OfflinePlace = {
  /** Absent on places kept before the full path was (refetched online). */
  brand?: string
  stream?: string
  project: string
  category: string
}

export type OfflineCardRecord = {
  /** `${scope}|${cardId}` */
  key: string
  scope: string
  cardId: string
  /** The server's card as last downloaded or synced. */
  base: Card
  /** The working copy, when it has unsynced edits; null when clean. */
  local: Card | null
  attachments: Record<string, OfflineAttachment>
  /** Unresolved merge conflicts and the server card they were found
   *  against. While any remain the card doesn't sync. */
  conflicts: OfflineConflict[]
  conflictServer: Card | null
  /** Choices made so far for `conflicts`, by conflict id. */
  resolutions: Record<string, ConflictChoice>
  markedAt: number
  fetchedAt: number
  syncedAt: number | null
  /** The last sync failure, shown on the card until a sync succeeds. */
  syncError: string | null
  /** Attachments the last sync had to keep both copies of. */
  keptBoth: string[]
  /** Its pins as last seen online; empty = unfiled (Inbox). Absent on
   *  records stored before places were kept. */
  places?: OfflinePlace[]
}

/** A cached read response, keyed by method and arguments. */
export type OfflineResponse = {
  /** `${scope}|${method}|${JSON args}` */
  key: string
  scope: string
  /** The card it belongs to; absent for board-level reads (card types…). */
  cardId?: string
  value: unknown
  at: number
}

export interface OfflineStorage {
  getCard(key: string): Promise<OfflineCardRecord | undefined>
  putCard(rec: OfflineCardRecord): Promise<void>
  deleteCard(key: string): Promise<void>
  listCards(scope: string): Promise<OfflineCardRecord[]>
  getResponse(key: string): Promise<OfflineResponse | undefined>
  putResponse(rec: OfflineResponse): Promise<void>
  /** Drop a card's cached reads, or the board-level ones when cardId is undefined. */
  deleteResponses(scope: string, cardId: string | undefined): Promise<void>
}

export const cardKey = (scope: string, cardId: string) => `${scope}|${cardId}`
export const responseKey = (scope: string, method: string, params: unknown[]) => `${scope}|${method}|${JSON.stringify(params)}`

// Structured clone keeps stored records independent of the objects the
// UI goes on mutating (Svelte proxies included — they don't clone).
function snapshot<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T
}

export class MemoryStorage implements OfflineStorage {
  private cards = new Map<string, OfflineCardRecord>()
  private responses = new Map<string, OfflineResponse>()

  async getCard(key: string) { const r = this.cards.get(key); return r && snapshot(r) }
  async putCard(rec: OfflineCardRecord) { this.cards.set(rec.key, snapshot(rec)) }
  async deleteCard(key: string) { this.cards.delete(key) }
  async listCards(scope: string) { return [...this.cards.values()].filter((r) => r.scope === scope).map(snapshot) }
  async getResponse(key: string) { const r = this.responses.get(key); return r && snapshot(r) }
  async putResponse(rec: OfflineResponse) { this.responses.set(rec.key, snapshot(rec)) }
  async deleteResponses(scope: string, cardId: string | undefined) {
    for (const [k, r] of this.responses) if (r.scope === scope && r.cardId === cardId) this.responses.delete(k)
  }
}

const DB_NAME = 'bruv-offline'
const DB_VERSION = 1

export class IdbStorage implements OfflineStorage {
  private db: Promise<IDBDatabase> | null = null

  private open(): Promise<IDBDatabase> {
    this.db ??= new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION)
      req.onupgradeneeded = () => {
        const db = req.result
        db.createObjectStore('cards', { keyPath: 'key' }).createIndex('scope', 'scope')
        db.createObjectStore('responses', { keyPath: 'key' }).createIndex('scope', 'scope')
      }
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
      req.onblocked = () => reject(new Error('offline storage is blocked by another open copy of BRUV'))
    })
    // A failed open shouldn't poison every later call.
    this.db.catch(() => { this.db = null })
    return this.db
  }

  private async run<T>(store: string, mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T> | void): Promise<T> {
    const db = await this.open()
    return new Promise<T>((resolve, reject) => {
      const tx = db.transaction(store, mode)
      const req = fn(tx.objectStore(store))
      tx.oncomplete = () => resolve(req ? req.result : (undefined as T))
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error ?? new Error('offline storage write aborted'))
    })
  }

  getCard(key: string) { return this.run<OfflineCardRecord | undefined>('cards', 'readonly', (s) => s.get(key)) }
  putCard(rec: OfflineCardRecord) { return this.run<void>('cards', 'readwrite', (s) => { s.put(snapshot(rec)) }) }
  deleteCard(key: string) { return this.run<void>('cards', 'readwrite', (s) => { s.delete(key) }) }
  listCards(scope: string) {
    return this.run<OfflineCardRecord[]>('cards', 'readonly', (s) => s.index('scope').getAll(scope))
  }
  getResponse(key: string) { return this.run<OfflineResponse | undefined>('responses', 'readonly', (s) => s.get(key)) }
  putResponse(rec: OfflineResponse) { return this.run<void>('responses', 'readwrite', (s) => { s.put(snapshot(rec)) }) }
  deleteResponses(scope: string, cardId: string | undefined) {
    return this.run<void>('responses', 'readwrite', (s) => {
      const req = s.index('scope').openCursor(scope)
      req.onsuccess = () => {
        const cursor = req.result
        if (!cursor) return
        if ((cursor.value as OfflineResponse).cardId === cardId) cursor.delete()
        cursor.continue()
      }
    })
  }
}

/** IndexedDB when the engine has it, else memory (cache lasts the session). */
export function defaultStorage(): OfflineStorage {
  return typeof indexedDB === 'undefined' ? new MemoryStorage() : new IdbStorage()
}
