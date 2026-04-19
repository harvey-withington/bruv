import { defineConfig } from 'vitest/config'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte({ hot: false })],
  // Force the browser build of Svelte inside jsdom, otherwise
  // @testing-library/svelte picks up the SSR entry and mount() throws
  // "lifecycle_function_unavailable".
  resolve: {
    conditions: ['browser'],
  },
  define: {
    'import.meta.env.VITE_BACKEND': JSON.stringify('mock'),
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['src/test-setup.ts'],
  },
})
