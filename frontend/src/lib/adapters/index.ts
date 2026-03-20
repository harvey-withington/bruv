import type { BackendAdapter } from '../types'

let _adapter: BackendAdapter

export async function initBackend(): Promise<BackendAdapter> {
  const mode = import.meta.env.VITE_BACKEND || 'wails'
  switch (mode) {
    case 'wails': {
      const { wailsAdapter } = await import('./wails')
      _adapter = wailsAdapter
      break
    }
    default:
      throw new Error(`Unknown backend adapter: ${mode}`)
  }
  return _adapter
}

export function getBackend(): BackendAdapter {
  if (!_adapter) throw new Error('Backend not initialised — call initBackend() first')
  return _adapter
}
