import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  define: {
    'import.meta.env.VITE_BACKEND': JSON.stringify(process.env.VITE_BACKEND || 'wails'),
  },
  server: {
    port: 5173,
    strictPort: true,
  }
})
