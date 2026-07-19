<script lang="ts">
  import type { Slide, DeckTheme, SlideDisplayRole } from '@shared/types'
  import { resolveSlideTemplate, entranceClass } from '@shared/slideTemplates'
  import { resolveContentType } from '@shared/slideContentTypes'

  // Generic, data-driven renderer: it reads the template's field→role map for
  // the slide's content type and renders each field by its display role.
  // Field values come from slide.values, or from a live binding the caller
  // resolves (desktop store / server-side for /present) — injected so this
  // component stays surface-agnostic. Media values resolve to a URL likewise.
  let {
    slide,
    deckTheme,
    resolveField,
    resolveMediaUrl,
  }: {
    slide: Slide
    deckTheme?: DeckTheme
    resolveField?: (slide: Slide, fieldKey: string) => string | undefined
    resolveMediaUrl?: (value: string) => string | undefined
  } = $props()

  const contentType = $derived(resolveContentType(slide.contentTypeId))
  const template = $derived(resolveSlideTemplate(slide.templateId, slide.contentTypeId))
  const animClass = $derived(entranceClass(template.entrance))

  type RenderItem = { role: SlideDisplayRole; type: string; value: string }

  function fieldValue(key: string): string {
    if (resolveField) {
      const v = resolveField(slide, key)
      if (v != null) return v
    }
    return slide.values?.[key] ?? ''
  }

  const items = $derived<RenderItem[]>(
    (template.fieldMap[slide.contentTypeId] ?? [])
      .map((m): RenderItem => {
        const type = contentType?.fields.find((f) => f.key === m.field)?.type ?? 'text'
        return { role: m.role, type, value: fieldValue(m.field) }
      })
      .filter((it) => it.value !== ''),
  )

  function mediaSrc(value: string): string | undefined {
    if (resolveMediaUrl) {
      const u = resolveMediaUrl(value)
      if (u) return u
    }
    return /^(https?:|data:|blob:)/.test(value) ? value : undefined
  }

  // Slides are content, not app chrome — stable dark-stage look regardless of
  // the app theme, overridable per deck/template via scoped CSS custom props.
  const stageStyle = $derived(
    [
      `--slide-bg:${deckTheme?.transparent ? 'transparent' : deckTheme?.backgroundColor ?? template.styles?.backgroundColor ?? '#0b0b12'}`,
      `--slide-fg:${deckTheme?.textColor ?? template.styles?.textColor ?? '#ffffff'}`,
      `--slide-accent:${deckTheme?.accentColor ?? template.styles?.accentColor ?? '#8b5cf6'}`,
      `--slide-anim-ms:${template.durationMs}ms`,
      deckTheme?.fontFamily ? `--slide-font:${deckTheme.fontFamily}` : '',
    ]
      .filter(Boolean)
      .join(';'),
  )
</script>

<div class="slide-stage" style={stageStyle}>
  {#key slide.id}
    <div class="slide-frame {animClass}">
      {#each items as item, i (i)}
        {#if item.role === 'heading'}
          <h1 class="r-heading">{item.value}</h1>
        {:else if item.role === 'subheading'}
          <p class="r-subheading">{item.value}</p>
        {:else if item.role === 'body'}
          <p class="r-body">{item.value}</p>
        {:else if item.role === 'quote'}
          <div class="r-quotemark">&ldquo;</div>
          <blockquote class="r-quote">{item.value}</blockquote>
        {:else if item.role === 'attribution'}
          <p class="r-attribution">— {item.value}</p>
        {:else if item.role === 'media'}
          {@const src = mediaSrc(item.value)}
          {#if src && item.type === 'video'}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video class="r-media" {src} autoplay muted loop playsinline></video>
          {:else if src}
            <img class="r-media" {src} alt="" />
          {:else}
            <div class="r-placeholder">{item.value}</div>
          {/if}
        {:else if item.role === 'caption'}
          <p class="r-caption">{item.value}</p>
        {/if}
      {/each}
      {#if items.length === 0}
        <p class="r-empty">—</p>
      {/if}
    </div>
  {/key}
</div>

<style>
  .slide-stage {
    --slide-font: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
    position: relative;
    width: 100%;
    height: 100%;
    background: var(--slide-bg);
    color: var(--slide-fg);
    font-family: var(--slide-font);
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    padding: 3rem;
    box-sizing: border-box;
  }
  .slide-frame {
    max-width: 60rem;
    width: 100%;
    text-align: center;
  }
  .r-heading {
    font-size: clamp(1.5rem, 4vw, 3rem);
    font-weight: 300;
    line-height: 1.25;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .r-subheading {
    font-size: clamp(1rem, 2.2vw, 1.6rem);
    font-weight: 300;
    opacity: 0.7;
    margin: 0.6rem 0 0;
    overflow-wrap: anywhere;
  }
  .r-body {
    font-size: clamp(1rem, 2vw, 1.4rem);
    opacity: 0.85;
    margin: 1rem 0 0;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  .r-quotemark {
    font-size: clamp(3rem, 8vw, 5rem);
    line-height: 0.5;
    opacity: 0.3;
    margin-bottom: 1.5rem;
    color: var(--slide-accent);
  }
  .r-quote {
    font-size: clamp(1.5rem, 4vw, 3rem);
    font-weight: 300;
    font-style: italic;
    line-height: 1.3;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .r-attribution {
    font-size: clamp(1rem, 1.6vw, 1.25rem);
    opacity: 0.7;
    margin: 1.5rem 0 0;
    overflow-wrap: anywhere;
  }
  .r-media {
    max-height: 70vh;
    max-width: 100%;
    border-radius: 8px;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
  }
  .r-caption {
    font-size: clamp(1rem, 2vw, 1.4rem);
    margin: 1rem 0 0;
    overflow-wrap: anywhere;
  }
  .r-placeholder {
    font-size: 1.2rem;
    opacity: 0.5;
    padding: 3rem;
    border: 1px dashed color-mix(in srgb, var(--slide-fg) 30%, transparent);
    border-radius: 8px;
  }
  .r-empty {
    opacity: 0.4;
    font-size: 2rem;
  }

  /* Entrance animations — re-fired via the {#key slide.id} wrapper. */
  .slide-anim-fadeIn { animation: slide-fadeIn var(--slide-anim-ms, 500ms) ease-out; }
  .slide-anim-zoomIn { animation: slide-zoomIn var(--slide-anim-ms, 500ms) ease-out; }
  .slide-anim-slideInLeft { animation: slide-inLeft var(--slide-anim-ms, 500ms) ease-out; }
  .slide-anim-slideInRight { animation: slide-inRight var(--slide-anim-ms, 500ms) ease-out; }
  .slide-anim-slideInUp { animation: slide-inUp var(--slide-anim-ms, 500ms) ease-out; }

  @keyframes slide-fadeIn { from { opacity: 0; } to { opacity: 1; } }
  @keyframes slide-zoomIn { from { opacity: 0; transform: scale(0.92); } to { opacity: 1; transform: scale(1); } }
  @keyframes slide-inLeft { from { opacity: 0; transform: translateX(-40px); } to { opacity: 1; transform: translateX(0); } }
  @keyframes slide-inRight { from { opacity: 0; transform: translateX(40px); } to { opacity: 1; transform: translateX(0); } }
  @keyframes slide-inUp { from { opacity: 0; transform: translateY(40px); } to { opacity: 1; transform: translateY(0); } }

  @media (prefers-reduced-motion: reduce) {
    .slide-frame { animation: none !important; }
  }
</style>
