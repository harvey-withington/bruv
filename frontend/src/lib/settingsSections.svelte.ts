// Load / save bookkeeping for a settings form made of independent
// sections (each backed by its own Get/Set RPC pair).
//
// The rule it enforces (2026-09-26): a section that failed to load is
// never saved. Loading everything with one Promise.all used to swallow a
// single failed RPC, leave the WHOLE form on hard-coded defaults, and let
// Save write those defaults over the user's real settings. Here each
// section loads, retries and saves on its own; Save only touches sections
// whose values actually came from the server.

export type SectionStatus = 'loading' | 'ready' | 'failed'

export interface SectionSpec {
  /** Fetch and apply the section's values to the form. */
  load: () => Promise<void>
  /** Persist the section's current values. */
  save: () => Promise<void>
}

export class SettingsSections<Id extends string> {
  status = $state({} as Record<Id, SectionStatus>)
  errors = $state({} as Record<Id, string>)

  constructor(private readonly specs: Record<Id, SectionSpec>) {
    for (const id of this.ids()) this.status[id] = 'loading'
  }

  private ids(): Id[] {
    return Object.keys(this.specs) as Id[]
  }

  /** Loads every section independently; one failure never blocks the rest. */
  loadAll(): Promise<void> {
    return Promise.all(this.ids().map(id => this.load(id))).then(() => {})
  }

  async load(id: Id): Promise<void> {
    this.status[id] = 'loading'
    try {
      await this.specs[id].load()
      this.status[id] = 'ready'
      delete this.errors[id]
    } catch (e: unknown) {
      this.errors[id] = e instanceof Error ? e.message : String(e)
      this.status[id] = 'failed'
    }
  }

  isReady(id: Id): boolean {
    return this.status[id] === 'ready'
  }

  get anyLoading(): boolean {
    return this.ids().some(id => this.status[id] === 'loading')
  }

  /** Saves the sections that loaded; returns the ids whose save failed. */
  async saveReady(): Promise<{ saved: Id[]; failed: Id[] }> {
    const ready = this.ids().filter(id => this.isReady(id))
    const results = await Promise.allSettled(ready.map(id => this.specs[id].save()))
    const saved: Id[] = []
    const failed: Id[] = []
    results.forEach((r, i) => {
      if (r.status === 'fulfilled') saved.push(ready[i])
      else {
        console.error(`settings: saving ${ready[i]} failed`, r.reason)
        failed.push(ready[i])
      }
    })
    return { saved, failed }
  }
}
