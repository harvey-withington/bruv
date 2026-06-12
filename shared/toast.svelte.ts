export type ToastType = 'info' | 'success' | 'error' | 'warning'

export type Toast = {
  id: string
  message: string
  type: ToastType
  dismissing?: boolean
}

export const toasts = $state<{ list: Toast[] }>({ list: [] })

const DISMISS_DURATION = 200 // matches CSS animation

function animateOut(id: string) {
  const toast = toasts.list.find(t => t.id === id)
  if (!toast || toast.dismissing) return
  toast.dismissing = true
  // Force reactivity
  toasts.list = [...toasts.list]
  setTimeout(() => {
    toasts.list = toasts.list.filter(t => t.id !== id)
  }, DISMISS_DURATION)
}

export function showToast(message: string, type: ToastType = 'info', durationMs = 4000) {
  const id = crypto.randomUUID()
  toasts.list = [...toasts.list, { id, message, type }]
  setTimeout(() => animateOut(id), durationMs)
}

export function dismissToast(id: string) {
  animateOut(id)
}
