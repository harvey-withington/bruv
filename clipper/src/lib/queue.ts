// Offline queue: clip jobs that couldn't finish wait in chrome.storage.local
// (media already embedded as base64, so nothing rots) and drain on a
// 1-minute alarm with per-job exponential backoff, or on an explicit Retry.
//
// Rules this module guarantees:
// - NOTHING is dropped silently. A job leaves the queue only by succeeding
//   or by the user discarding it in the popup; every failure stays listed
//   with its error.
// - The background service worker is the ONLY writer and drainer (the
//   popup messages it — see QueueDrainMessage). Within the worker, every
//   storage mutation runs through one in-memory lock, and drains are
//   single-flight, so no two drains ever run one job twice.
// - Storage is per job: the immutable payload under its own key, the small
//   mutable state (attempts, error, progress, lease) under another. Updates
//   touch one job's state, never a whole-queue snapshot — a clip enqueued
//   during a drain can't be overwritten by it.
// - Progress is persisted after every step, so a retry RESUMES the job
//   (see clip.ts executeJob) instead of creating a second card.

import type { ClipJob, ClipProgress, ClipperSettings, QueueDrainResult, QueueEntry } from './types'
import { errorText, isNetworkError } from './api'
import { executeJob, jobTitle } from './clip'

const IDS_KEY = 'bruv_queue_ids'
// Pre-2026-09-30 builds kept the whole queue as one array under this key.
const LEGACY_KEY = 'bruv_queue'
const jobKey = (id: string): string => `bruv_qjob:${id}`
const stateKey = (id: string): string => `bruv_qstate:${id}`

const MINUTE = 60_000
const MAX_BACKOFF_MS = 60 * MINUTE
// A drain holds a job's lease while executing it and renews it on every
// persisted step. It only matters if the worker dies mid-job: the lease
// then lapses on its own and the next drain resumes from the progress.
const LEASE_MS = 5 * MINUTE

type LegacyClipJob = ClipJob & { attempts?: number; lastError?: string }

// backoffMs: 1, 2, 4 … minutes, capped at an hour — forever, never expiring.
export function backoffMs(attempts: number): number {
  return Math.min(MAX_BACKOFF_MS, MINUTE * 2 ** Math.max(0, attempts - 1))
}

// --- reads (safe from any extension context) --------------------------------

async function readIds(): Promise<string[]> {
  const got = await chrome.storage.local.get(IDS_KEY)
  return (got[IDS_KEY] as string[] | undefined) ?? []
}

async function readState(id: string): Promise<QueueEntry | undefined> {
  const key = stateKey(id)
  const got = await chrome.storage.local.get(key)
  return got[key] as QueueEntry | undefined
}

function legacyEntry(job: LegacyClipJob): QueueEntry {
  return {
    id: job.id,
    createdAt: job.createdAt,
    label: jobTitle(job),
    attempts: job.attempts ?? 0,
    nextAttemptAt: 0,
    lastError: job.lastError,
    progress: {},
  }
}

// listQueue returns every queued job's state, oldest first — without
// loading any media payloads, so the popup can list a queue holding
// hundreds of MB.
export async function listQueue(): Promise<QueueEntry[]> {
  const ids = await readIds()
  const got = await chrome.storage.local.get([...ids.map(stateKey), LEGACY_KEY])
  const entries = ids.map((id) => got[stateKey(id)] as QueueEntry | undefined).filter((e): e is QueueEntry => !!e)
  // Not yet migrated (the worker migrates on its next queue operation) —
  // still listed, so an upgrade never hides a waiting clip.
  const legacy = (got[LEGACY_KEY] as LegacyClipJob[] | undefined) ?? []
  return [...legacy.map(legacyEntry), ...entries]
}

// isQueueStorageKey lets the popup re-render when (and only when) the
// queue changed.
export function isQueueStorageKey(key: string): boolean {
  return key === IDS_KEY || key === LEGACY_KEY || key.startsWith('bruv_qstate:')
}

// --- mutations (service worker only) ---------------------------------------

let lockChain: Promise<unknown> = Promise.resolve()

function locked<T>(fn: () => Promise<T>): Promise<T> {
  const run = lockChain.then(fn)
  lockChain = run.catch(() => undefined)
  return run
}

// Moves a legacy whole-array queue onto per-job keys. Caller holds the lock.
async function migrateLegacy(): Promise<void> {
  const got = await chrome.storage.local.get(LEGACY_KEY)
  const legacy = (got[LEGACY_KEY] as LegacyClipJob[] | undefined) ?? []
  if (legacy.length === 0) return
  const ids = await readIds()
  const items: Record<string, unknown> = {}
  for (const old of legacy) {
    const { attempts: _attempts, lastError: _lastError, ...job } = old
    items[jobKey(job.id)] = job
    items[stateKey(job.id)] = legacyEntry(old)
    if (!ids.includes(job.id)) ids.push(job.id)
  }
  items[IDS_KEY] = ids
  await chrome.storage.local.set(items)
  await chrome.storage.local.remove(LEGACY_KEY)
}

// enqueue adds a job, carrying whatever progress a failed live attempt
// made (so the retry finishes that card rather than making another).
export function enqueue(job: ClipJob, progress: ClipProgress = {}, lastError?: string): Promise<void> {
  return locked(async () => {
    await migrateLegacy()
    const ids = await readIds()
    const entry: QueueEntry = {
      id: job.id,
      createdAt: job.createdAt,
      label: jobTitle(job),
      attempts: 1,
      nextAttemptAt: Date.now() + backoffMs(1),
      lastError,
      progress,
    }
    await chrome.storage.local.set({
      [jobKey(job.id)]: job,
      [stateKey(job.id)]: entry,
      [IDS_KEY]: ids.includes(job.id) ? ids : [...ids, job.id],
    })
  })
}

async function removeJob(id: string): Promise<void> {
  const ids = await readIds()
  await chrome.storage.local.set({ [IDS_KEY]: ids.filter((x) => x !== id) })
  await chrome.storage.local.remove([jobKey(id), stateKey(id)])
}

// discardJobs removes the given jobs (all of them when `ids` is absent).
// Only ever the user's explicit decision.
export function discardJobs(ids?: string[]): Promise<void> {
  return locked(async () => {
    await migrateLegacy()
    for (const id of ids ?? (await readIds())) await removeJob(id)
  })
}

// updateState merges `patch` into a job's state — a no-op when the job was
// discarded meanwhile, so a finishing drain can't resurrect it.
function updateState(id: string, patch: (entry: QueueEntry) => QueueEntry): Promise<void> {
  return locked(async () => {
    const entry = await readState(id)
    if (entry) await chrome.storage.local.set({ [stateKey(id)]: patch(entry) })
  })
}

// claim takes the lease on a job that is due (or on any job when forced),
// returning its payload + state; null to skip it.
function claim(id: string, force: boolean): Promise<{ job: ClipJob | undefined; entry: QueueEntry } | null> {
  return locked(async () => {
    const entry = await readState(id)
    if (!entry) return null
    const now = Date.now()
    if ((entry.leaseUntil ?? 0) > now) return null
    if (!force && entry.nextAttemptAt > now) return null
    const leased: QueueEntry = { ...entry, leaseUntil: now + LEASE_MS }
    await chrome.storage.local.set({ [stateKey(id)]: leased })
    const got = await chrome.storage.local.get(jobKey(id))
    return { job: got[jobKey(id)] as ClipJob | undefined, entry: leased }
  })
}

function succeed(id: string): Promise<void> {
  return locked(() => removeJob(id))
}

// fail records a failed attempt. An unreachable server isn't an error to
// show — the job is simply waiting — so only other failures set lastError.
function fail(id: string, progress: ClipProgress, err: unknown): Promise<void> {
  const error = isNetworkError(err) ? undefined : errorText(err)
  return updateState(id, (entry) => {
    const attempts = entry.attempts + 1
    return {
      ...entry,
      attempts,
      nextAttemptAt: Date.now() + backoffMs(attempts),
      lastError: error,
      progress,
      leaseUntil: undefined,
    }
  })
}

async function drainOnce(s: ClipperSettings, force: boolean): Promise<QueueDrainResult> {
  await locked(migrateLegacy)
  let done = 0
  for (const id of await readIds()) {
    const claimed = await claim(id, force)
    if (!claimed) continue
    if (!claimed.job) {
      // Payload lost (storage cleared underneath us) — keep it listed so
      // the user sees it and can discard it; never drop it quietly.
      await fail(id, claimed.entry.progress, new Error(chrome.i18n.getMessage('queue_err_missing_data')))
      continue
    }
    const progress: ClipProgress = { ...claimed.entry.progress }
    try {
      await executeJob(s, claimed.job, progress, (p) =>
        updateState(id, (entry) => ({ ...entry, progress: { ...p }, leaseUntil: Date.now() + LEASE_MS })),
      )
      await succeed(id)
      done++
    } catch (err) {
      await fail(id, progress, err)
    }
  }
  return { done, remaining: (await readIds()).length }
}

// Drains are single-flight: the alarm joins a drain already running; a
// forced (user) drain arriving mid-drain runs once more right after it, so
// a job the running pass skipped for backoff still gets its forced try.
let running: Promise<QueueDrainResult> | null = null
let forcedNext: Promise<QueueDrainResult> | null = null

export function drainQueue(s: ClipperSettings, force = false): Promise<QueueDrainResult> {
  if (!running) {
    running = drainOnce(s, force).finally(() => {
      running = null
    })
    return running
  }
  if (!force) return running
  forcedNext ??= running
    .catch(() => undefined)
    .then(() => {
      forcedNext = null
      return drainQueue(s, true)
    })
  return forcedNext
}
