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

// A thrown error's type name ("Error:", "TypeError:", "RPCError:") means
// nothing to the user; callers often pass String(e), which carries it.
const ERROR_TYPE_PREFIX = /^(?:[A-Z]\w*)?Error:\s*/

export function showToast(message: string, type: ToastType = 'info', durationMs = 4000) {
  const id = crypto.randomUUID()
  toasts.list = [...toasts.list, { id, message: message.replace(ERROR_TYPE_PREFIX, ''), type }]
  setTimeout(() => animateOut(id), durationMs)
}

export function dismissToast(id: string) {
  animateOut(id)
}
