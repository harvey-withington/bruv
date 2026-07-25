// Offline queue: jobs that couldn't reach the server wait in
// chrome.storage.local (media already embedded as base64, so nothing rots)
// and drain on a 1-minute alarm or an explicit retry. Network errors keep
// the job; business errors keep it too but stamp lastError so the popup can
// show what's stuck instead of silently eating a clip.

import type { ClipJob, ClipperSettings } from './types'
import { isNetworkError } from './api'
import { executeJob } from './clip'

const QUEUE_KEY = 'bruv_queue'
const MAX_ATTEMPTS = 50

export async function listQueue(): Promise<ClipJob[]> {
  const got = await chrome.storage.local.get(QUEUE_KEY)
  return (got[QUEUE_KEY] as ClipJob[] | undefined) ?? []
}

async function saveQueue(jobs: ClipJob[]): Promise<void> {
  await chrome.storage.local.set({ [QUEUE_KEY]: jobs })
}

export async function enqueue(job: ClipJob): Promise<void> {
  const jobs = await listQueue()
  jobs.push(job)
  await saveQueue(jobs)
}

export async function clearQueue(): Promise<void> {
  await chrome.storage.local.remove(QUEUE_KEY)
}

export type DrainResult = { done: number; remaining: number }

// drainQueue attempts every queued job once, oldest first. Jobs that
// succeed (or exceed MAX_ATTEMPTS) leave the queue; the rest stay with an
// updated attempt count + error.
export async function drainQueue(s: ClipperSettings): Promise<DrainResult> {
  const jobs = await listQueue()
  if (jobs.length === 0) return { done: 0, remaining: 0 }

  const remaining: ClipJob[] = []
  let done = 0
  for (const job of jobs) {
    try {
      await executeJob(s, job)
      done++
    } catch (err) {
      const attempts = job.attempts + 1
      if (attempts >= MAX_ATTEMPTS) continue // give up silently after weeks of retries
      remaining.push({
        ...job,
        attempts,
        lastError: isNetworkError(err) ? undefined : err instanceof Error ? err.message : String(err),
      })
    }
  }
  await saveQueue(remaining)
  return { done, remaining: remaining.length }
}
