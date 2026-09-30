// Two-step destructive confirm for popup buttons: the first click ARMS the
// button (its text turns into the question), a second click within the
// window confirms, and the arming lapses on its own. The extension has no
// dialog layer and native confirm() is banned project-wide, so every
// destructive popup action goes through this.

const DEFAULT_ARM_MS = 3000

export type ArmButtonOptions = {
  idleText: string
  armedText: string
  // Tooltip at rest (e.g. for an icon-only "×" button).
  idleTitle?: string
  onConfirm: () => void
  armMs?: number
}

export function armButton(btn: HTMLButtonElement, opts: ArmButtonOptions): void {
  let armed = false
  let timer: ReturnType<typeof setTimeout> | undefined
  const disarm = (): void => {
    armed = false
    btn.classList.remove('armed')
    btn.textContent = opts.idleText
    btn.title = opts.idleTitle ?? ''
  }
  disarm()
  btn.addEventListener('click', () => {
    clearTimeout(timer)
    if (!armed) {
      armed = true
      btn.classList.add('armed')
      btn.textContent = opts.armedText
      btn.title = opts.armedText
      timer = setTimeout(disarm, opts.armMs ?? DEFAULT_ARM_MS)
      return
    }
    disarm()
    opts.onConfirm()
  })
}
