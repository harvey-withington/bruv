<script lang="ts">
  import type { DocumentFormat } from '@shared/documentFormats'

  // The rendered document. Rendering is debounced behind typing so a long
  // manuscript doesn't re-render per keystroke; the first paint is
  // immediate so opening in preview mode never shows a blank pane.
  let { text, format, name }: {
    text: string
    format: DocumentFormat
    name: string
  } = $props()

  let html = $state('')
  let first = true
  $effect(() => {
    const t = text
    if (first) {
      first = false
      html = format.render(t, { name })
      return
    }
    const h = setTimeout(() => { html = format.render(t, { name }) }, 150)
    return () => clearTimeout(h)
  })
</script>

<div class="preview-pane">
  {#if format.styles}
    {@html `<style>${format.styles}</style>`}
  {/if}
  <div class="doc-preview {format.id}" class:markdown-content={format.id === 'markdown'}>{@html html}</div>
</div>

<style>
  .preview-pane {
    height: 100%;
    min-height: 0;
    min-width: 0;
    overflow: auto;
    padding: 1rem 1.5rem 3rem;
    background: var(--bg-surface);
  }
  .doc-preview {
    max-width: 46rem;
    margin: 0 auto;
  }
  .doc-preview :global(.doc-plain) {
    margin: 0;
    font-size: 0.85rem;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--text-body);
  }
</style>
