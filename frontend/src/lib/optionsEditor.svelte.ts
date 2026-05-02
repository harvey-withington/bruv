import type { BlockMeta } from '@shared/types'

export type OptionsEditorConfig = {
  visible: boolean
  title: string
  blockType: 'select' | 'radio' | 'checkbox_group' | ''
  options: string[]
  meta: BlockMeta
  resolve: ((result: { options: string[]; meta: BlockMeta } | null) => void) | null
}

export const optionsEditorState = $state<OptionsEditorConfig>({
  visible: false,
  title: '',
  blockType: '',
  options: [],
  meta: {},
  resolve: null,
})

/**
 * Open the options editor dialog. Returns the updated options+meta, or null if cancelled.
 */
export function showOptionsEditor(
  title: string,
  blockType: 'select' | 'radio' | 'checkbox_group',
  options: string[],
  meta: BlockMeta,
): Promise<{ options: string[]; meta: BlockMeta } | null> {
  return new Promise(resolve => {
    optionsEditorState.visible = true
    optionsEditorState.title = title
    optionsEditorState.blockType = blockType
    optionsEditorState.options = [...options]
    optionsEditorState.meta = { ...meta }
    optionsEditorState.resolve = resolve
  })
}

export function resolveOptionsEditor(result: { options: string[]; meta: BlockMeta } | null) {
  optionsEditorState.visible = false
  optionsEditorState.resolve?.(result)
  optionsEditorState.resolve = null
}
