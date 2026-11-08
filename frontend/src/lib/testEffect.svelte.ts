// Test helper: runs `fn` as an $effect (tests are plain .ts, where runes
// don't compile) and counts how often it runs. Returns the count and a
// cleanup.

export function countEffectRuns(fn: () => void): { runs: () => number; stop: () => void } {
  let runs = 0
  const stop = $effect.root(() => {
    $effect(() => {
      runs++
      fn()
    })
  })
  return { runs: () => runs, stop }
}
