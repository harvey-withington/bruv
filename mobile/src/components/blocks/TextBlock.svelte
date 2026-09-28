<script lang="ts">
  import { t } from '../../lib/i18n.svelte'
  import type { Block } from '@shared/types'
  import { asString, withValue } from './narrow'
  import EditableDescription from '../EditableDescription.svelte'

  // A text block is tap-to-edit markdown, exactly like the card's
  // description: rendered prose at rest, an autosizing textarea while
  // editing, ✓ Done / tap-away commits and returns to prose. It used to
  // open in an always-on editor with an eye toggle for preview
  // (ruling 2026-09-21: blocks start in display mode, and Done means
  // done). EditableDescription owns the whole edit session — draft,
  // keyboard contract, mentions, external-edit sync (the prop is
  // re-read whenever the block isn't being edited).

  let {
    block,
    onChange,
  }: {
    block: Block
    onChange: (next: Block) => void
  } = $props()
</script>

<EditableDescription
  value={asString(block.value)}
  placeholder={t('block.text.placeholder')}
  editLabel={t('block.text.edit')}
  onSave={(next) => onChange(withValue(block, next))}
/>
