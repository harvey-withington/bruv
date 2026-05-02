import { copyFileSync, mkdirSync, statSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const root = resolve(__dirname, '..')

// Vendor assets that ship in node_modules but cannot be imported via Vite
// (e.g. blocked by a package's `exports` map). Copy into public/ so they are
// served from the app root and stay version-locked to the installed package.
const assets = [
  {
    from: resolve(root, 'node_modules/@lottiefiles/dotlottie-web/dist/dotlottie-player.wasm'),
    to: resolve(root, 'public/dotlottie-player.wasm'),
  },
]

function isStale(from, to) {
  let toTime
  try { toTime = statSync(to).mtimeMs } catch { return true }
  const fromTime = statSync(from).mtimeMs
  return fromTime > toTime
}

let copied = 0
for (const { from, to } of assets) {
  try {
    statSync(from)
  } catch {
    console.error(`Vendor asset missing: ${from}`)
    process.exit(1)
  }
  if (!isStale(from, to)) continue
  mkdirSync(dirname(to), { recursive: true })
  copyFileSync(from, to)
  copied++
}

if (copied === 0) {
  console.log('Vendor assets up to date, skipping.')
} else {
  console.log(`Copied ${copied} vendor asset${copied === 1 ? '' : 's'}.`)
}
