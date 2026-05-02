import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))

// The Go backend serves the mobile bundle at /m/. Vite's `base` puts
// every asset URL under that prefix so direct loads (Add to Home Screen,
// share_target deep links) resolve without 404s.
export default defineConfig({
  base: '/m/',
  plugins: [svelte()],
  resolve: {
    alias: {
      '@shared': resolve(__dirname, '../shared'),
    },
  },
  server: {
    port: 5174,
    strictPort: true,
  },
})
