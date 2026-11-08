// A load that must end: the promise's outcome, or `TimeoutError` once
// `ms` pass — so a view waiting on it always leaves its loading state
// and can show what went wrong instead of spinning forever.

export class TimeoutError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'TimeoutError'
  }
}

export function withTimeout<T>(promise: Promise<T>, ms: number, message: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new TimeoutError(message)), ms)
  })
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer))
}

/** How long a card may take to open before its view gives up and says so. */
export const CARD_LOAD_TIMEOUT_MS = 20_000
