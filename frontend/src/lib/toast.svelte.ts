export type ToastType = 'info' | 'success' | 'error' | 'warning'

export type Toast = {
  id: string
  message: string
  type: ToastType
}

export const toasts = $state<{ list: Toast[] }>({ list: [] })

export function showToast(message: string, type: ToastType = 'info', durationMs = 4000) {
  const id = crypto.randomUUID()
  toasts.list = [...toasts.list, { id, message, type }]
  setTimeout(() => {
    toasts.list = toasts.list.filter(t => t.id !== id)
  }, durationMs)
}

export function dismissToast(id: string) {
  toasts.list = toasts.list.filter(t => t.id !== id)
}
