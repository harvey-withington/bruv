// Where keyboard focus really is. document.activeElement stops at a
// shadow host, so while the user types in an embedded element's field
// (UNCLI's <uncli-chat> message box) it is the element itself, and a
// global shortcut would take the keystroke. These look inside open shadow
// roots.

/** The focused element, inside any open shadow roots. */
export function deepActiveElement(root: Document | ShadowRoot = document): Element | null {
  let el = root.activeElement
  while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement
  return el
}

/** Is the user typing: focus in an input, textarea, select or editable
 *  element, shadow roots included? Global single-key shortcuts check
 *  this first. */
export function isTyping(root: Document | ShadowRoot = document): boolean {
  const el = deepActiveElement(root)
  if (!el) return false
  const tag = el.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || (el as HTMLElement).isContentEditable === true
}
