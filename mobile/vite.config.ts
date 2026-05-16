import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))

// The Go backend serves the mobile bundle at /m/. Vite's `base` puts
// every asset URL under that prefix so direct loads (Add to Home Screen,
// share_target deep links) resolve without 404s.
//
// In dev, the PWA assumes same-origin enrolment + RPC (the production
// Go server hosts both UI and API). Vite alone has no backend, so we
// proxy the backend paths to bruv-server on 9870 — that keeps the
// "same-origin" assumption true from the browser's point of view.
// VITE_BACKEND lets the operator override the target (e.g. a Tailscale
// hostname for testing against a remote dev server).
const backendTarget = process.env.VITE_BACKEND ?? 'http://127.0.0.1:9870'

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
    proxy: {
      '/auth':    { target: backendTarget, changeOrigin: true },
      '/repos':   { target: backendTarget, changeOrigin: true, ws: true },
      '/server':  { target: backendTarget, changeOrigin: true },
      '/pair':    { target: backendTarget, changeOrigin: true },
      '/healthz': { target: backendTarget, changeOrigin: true },
      '/version': { target: backendTarget, changeOrigin: true },
      // The Go server synthesises /m/manifest.webmanifest (mobileManifestHandler).
      // Vite doesn't, so without this the browser pulls back HTML/empty
      // and complains "Manifest: Line 1, column 1, Syntax error." in the
      // console. Cosmetic in dev; proxy it so install/share-target/
      // theme-color work the same as the production single-binary build.
      '/m/manifest.webmanifest': { target: backendTarget, changeOrigin: true },
    },
  },
})
