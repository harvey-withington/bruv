import { GetUIPreferences, SetUIPreferences } from '@shared/api'
import type { DocumentLayout } from '@shared/types'
import type { DocumentFormatId } from '@shared/documentFormats'

// Per-device editor preferences (client zone — ui_preferences.json): the
// edit / split / preview layout remembered per format, and whether the
// outline pane is open. Loaded once per session on first editor open;
// writes are partial merges so they never clobber the Settings dialog's
// fields.

const LAYOUTS: readonly DocumentLayout[] = ['edit', 'split', 'preview']
const DEFAULT_LAYOUT: DocumentLayout = 'split'

const prefs = $state<{ layouts: Record<string, DocumentLayout>; outline: boolean }>({ layouts: {}, outline: true })
let loaded: Promise<void> | null = null

function isLayout(v: unknown): v is DocumentLayout {
  return typeof v === 'string' && (LAYOUTS as readonly string[]).includes(v)
}

/** Idempotent; the first call fetches, later calls await the same load. */
export function loadDocumentPrefs(): Promise<void> {
  if (!loaded) {
    loaded = GetUIPreferences().then((ui) => {
      const layouts: Record<string, DocumentLayout> = {}
      for (const [k, v] of Object.entries(ui.document_layouts ?? {})) {
        if (isLayout(v)) layouts[k] = v
      }
      prefs.layouts = layouts
      prefs.outline = ui.document_outline ?? true
    })
  }
  return loaded
}

export function documentLayout(format: DocumentFormatId): DocumentLayout {
  return prefs.layouts[format] ?? DEFAULT_LAYOUT
}

export function outlineShown(): boolean {
  return prefs.outline
}

/** Persists in the background; the caller surfaces a failed write. */
export function setDocumentLayout(format: DocumentFormatId, layout: DocumentLayout): Promise<void> {
  prefs.layouts = { ...prefs.layouts, [format]: layout }
  return SetUIPreferences({ document_layouts: prefs.layouts })
}

export function setOutlineShown(shown: boolean): Promise<void> {
  prefs.outline = shown
  return SetUIPreferences({ document_outline: shown })
}
