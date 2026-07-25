// Build script: bundles the three MV3 entry points and copies static assets
// into dist/. esbuild directly (no Vite) because MV3 needs one IIFE bundle
// for the content script and plain single-file bundles for the rest — no
// code-splitting, no hashed filenames, nothing a manifest can't reference.
import { build } from 'esbuild'
import { cpSync, mkdirSync, rmSync } from 'node:fs'

rmSync('dist', { recursive: true, force: true })
mkdirSync('dist', { recursive: true })

const common = {
  bundle: true,
  minify: false,
  sourcemap: false,
  target: 'chrome110',
  logLevel: 'info',
}

// Background service worker (ESM — manifest declares "type": "module").
await build({ ...common, entryPoints: ['src/background.ts'], outfile: 'dist/background.js', format: 'esm' })
// Content script — classic script world, must be a self-contained IIFE.
await build({ ...common, entryPoints: ['src/content.ts'], outfile: 'dist/content.js', format: 'iife' })
// Popup + options pages.
await build({ ...common, entryPoints: ['src/popup/popup.ts'], outfile: 'dist/popup/popup.js', format: 'iife' })
await build({ ...common, entryPoints: ['src/options/options.ts'], outfile: 'dist/options/options.js', format: 'iife' })

cpSync('manifest.json', 'dist/manifest.json')
cpSync('src/popup/popup.html', 'dist/popup/popup.html')
cpSync('src/options/options.html', 'dist/options/options.html')
cpSync('src/content.css', 'dist/content.css')
cpSync('_locales', 'dist/_locales', { recursive: true })
cpSync('icons', 'dist/icons', { recursive: true })

console.log('clipper built → dist/ (load unpacked from there)')
