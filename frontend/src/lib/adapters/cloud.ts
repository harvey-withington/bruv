import type { BackendAdapter } from '../types'

// TODO: Implement cloud/SaaS adapter (REST/gRPC calls, OAuth, WebSocket events, etc.)
export const cloudAdapter: BackendAdapter = new Proxy({} as BackendAdapter, {
  get(_, prop) {
    throw new Error(`Cloud adapter not implemented: ${String(prop)}`)
  },
})
