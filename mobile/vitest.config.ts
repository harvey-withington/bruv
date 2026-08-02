import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

// Mirrors frontend/vitest.config.ts — same jsdom + browser-conditions
// setup, so a test written for one surface reads the same on the other.
// Kept separate from vite.config.ts because the dev config carries the
// PWA proxy and base path, neither of which belong in a test run.

const __dirname = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [svelte({ hot: false })],
  // Force the browser build of Svelte inside jsdom, otherwise
  // @testing-library/svelte picks up the SSR entry and mount() throws
  // "lifecycle_function_unavailable".
  resolve: {
    conditions: ['browser'],
    alias: {
      '@shared': resolve(__dirname, '../shared'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['src/test-setup.ts'],
  },
})
