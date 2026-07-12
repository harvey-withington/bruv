// Guarded dependency bootstrap for a JS package root.
//
// Git worktrees only materialize tracked files, and node_modules is
// gitignored — so every fresh worktree starts without dependencies and
// anything that reads node_modules (e.g. frontend's copy-vendor-assets
// predev hook) fails until they're installed. This script is wired as a
// preLaunchTask so a new worktree "just works" on first F5.
//
// It installs ONLY when node_modules is absent: the common case (deps
// already present) is a near-instant no-op with no network dependency,
// and we never mutate the lockfile on a normal launch. When it does run,
// it prefers `npm ci` (deterministic, lockfile-exact) and falls back to
// `npm install` when no lockfile exists.
//
// Usage: node scripts/ensure-deps.mjs <package-dir> [<package-dir> ...]
//        (paths are resolved relative to the repo root)

import { existsSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'
import { spawnSync } from 'child_process'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const targets = process.argv.slice(2)

if (targets.length === 0) {
  console.error('ensure-deps: no package directory given')
  process.exit(1)
}

const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'

for (const target of targets) {
  const dir = resolve(root, target)

  if (existsSync(resolve(dir, 'node_modules'))) {
    console.log(`ensure-deps: ${target} up to date, skipping.`)
    continue
  }

  const hasLock = existsSync(resolve(dir, 'package-lock.json'))
  const cmd = hasLock ? 'ci' : 'install'
  console.log(`ensure-deps: installing ${target} deps (npm ${cmd})...`)

  const result = spawnSync(npm, [cmd], { cwd: dir, stdio: 'inherit' })
  if (result.status !== 0) {
    console.error(`ensure-deps: npm ${cmd} failed in ${target}`)
    process.exit(result.status ?? 1)
  }
}
