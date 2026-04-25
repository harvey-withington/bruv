import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  define: {
    // Default to 'cloud' — the only remaining adapter. The legacy
    // 'wails' adapter was deleted in phase-4 cleanup; the selector
    // still maps any unknown value to cloud with a warning so a
    // stale VITE_BACKEND env doesn't crash the app.
    'import.meta.env.VITE_BACKEND': JSON.stringify(process.env.VITE_BACKEND || 'cloud'),
  },
  server: {
    port: 5173,
    strictPort: true,
    watch: {
      ignored: ['**/wailsjs/**'],
    },
  }
})
