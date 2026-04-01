type ConfirmState = {
  visible: boolean
  message: string
  resolve: ((value: boolean) => void) | null
}

export const confirmState = $state<ConfirmState>({
  visible: false,
  message: '',
  resolve: null,
})

export function showConfirm(message: string): Promise<boolean> {
  return new Promise(resolve => {
    confirmState.visible = true
    confirmState.message = message
    confirmState.resolve = resolve
  })
}

export function resolveConfirm(value: boolean) {
  confirmState.visible = false
  confirmState.resolve?.(value)
  confirmState.resolve = null
}
