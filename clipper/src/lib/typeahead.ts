// Combobox behaviour shared by the extension's two pickers (options'
// pin target, the popup's deck target) — BRUV's picker rules in one
// place: the list PRE-POPULATES on focus (an empty picker is capture
// friction), typing filters, ArrowUp/Down + Enter select, Escape
// reverts, and the committed selection is what the input shows at rest.
//
// The chosen value lives in the input itself with an inline clear (×)
// rather than a separate "current selection" line plus a Clear button —
// same shape as every picker in the app proper.

export type TypeaheadEntry = {
  id: string
  label: string
  /** Secondary line (breadcrumb/path), rendered muted under the label. */
  sub?: string
  /** Rendered in accent — for non-item choices like "Inbox". */
  special?: boolean
}

export type TypeaheadConfig = {
  input: HTMLInputElement
  list: HTMLUListElement
  /** Inline clear button; hidden automatically while the input is empty. */
  clearBtn?: HTMLButtonElement
  /** Entries for a query; '' means "focus, nothing typed yet". */
  entries: (query: string) => TypeaheadEntry[] | Promise<TypeaheadEntry[]>
  onSelect: (entry: TypeaheadEntry) => void | Promise<void>
  /** Omit to hide the clear affordance entirely. */
  onClear?: () => void | Promise<void>
  emptyLabel?: string
  debounceMs?: number
}

export type TypeaheadHandle = {
  /** Set the committed selection's display text (e.g. after loading). */
  setValue(label: string): void
  close(): void
}

export function typeahead(cfg: TypeaheadConfig): TypeaheadHandle {
  const { input, list } = cfg
  const debounceMs = cfg.debounceMs ?? 250

  // The last COMMITTED label — what the input reverts to when the user
  // types then bails (blur/Escape) without picking anything.
  let committed = input.value
  let activeIdx = -1
  let current: TypeaheadEntry[] = []
  let seq = 0
  let debounce: number | undefined

  function syncClear(): void {
    if (cfg.clearBtn) cfg.clearBtn.hidden = !cfg.onClear || input.value.trim() === ''
  }

  function close(): void {
    list.hidden = true
    list.innerHTML = ''
    activeIdx = -1
    input.setAttribute('aria-expanded', 'false')
  }

  function render(): void {
    list.innerHTML = ''
    if (current.length === 0) {
      if (!cfg.emptyLabel) {
        close()
        return
      }
      const li = document.createElement('li')
      li.className = 'empty'
      li.textContent = cfg.emptyLabel
      list.appendChild(li)
      list.hidden = false
      input.setAttribute('aria-expanded', 'true')
      return
    }
    current.forEach((entry, i) => {
      const li = document.createElement('li')
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.setAttribute('role', 'option')
      btn.className = (entry.special ? 'special' : '') + (i === activeIdx ? ' active' : '')
      const label = document.createElement('span')
      label.textContent = entry.label
      btn.appendChild(label)
      if (entry.sub) {
        const sub = document.createElement('span')
        sub.className = 'path'
        sub.textContent = entry.sub
        btn.appendChild(sub)
      }
      // pointerdown, not click — selection must beat the input's blur.
      btn.addEventListener('pointerdown', (e) => {
        e.preventDefault()
        void commit(entry)
      })
      li.appendChild(btn)
      list.appendChild(li)
    })
    list.hidden = false
    input.setAttribute('aria-expanded', 'true')
  }

  async function load(query: string): Promise<void> {
    const mine = ++seq
    const entries = await cfg.entries(query)
    if (mine !== seq) return // a newer keystroke already won
    current = entries
    activeIdx = -1
    render()
  }

  async function commit(entry: TypeaheadEntry): Promise<void> {
    committed = entry.label
    input.value = entry.label
    close()
    syncClear()
    await cfg.onSelect(entry)
  }

  function revert(): void {
    input.value = committed
    close()
    syncClear()
  }

  input.addEventListener('focus', () => {
    input.select()
    void load('')
  })

  input.addEventListener('input', () => {
    syncClear()
    clearTimeout(debounce)
    const q = input.value
    debounce = setTimeout(() => void load(q), debounceMs) as unknown as number
  })

  input.addEventListener('blur', () => {
    // Delay so a pointerdown on a result lands first.
    setTimeout(revert, 120)
  })

  input.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (current.length === 0) return
      activeIdx = (activeIdx + (e.key === 'ArrowDown' ? 1 : -1) + current.length) % current.length
      render()
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const entry = activeIdx >= 0 ? current[activeIdx] : current[0]
      if (entry) {
        void commit(entry)
        input.blur()
      }
    } else if (e.key === 'Escape') {
      e.preventDefault()
      revert()
    }
  })

  cfg.clearBtn?.addEventListener('click', () => {
    void (async () => {
      committed = ''
      input.value = ''
      close()
      syncClear()
      await cfg.onClear?.()
    })()
  })

  syncClear()

  return {
    setValue(label: string): void {
      committed = label
      input.value = label
      syncClear()
    },
    close,
  }
}
