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
  const layout = $derived(template.layout ?? 'stack')

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
    return /^(https?:|data:|blob:|\/)/.test(value) ? value : undefined
  }

  // --- post-card layout ---
  // Partition items by role: header takes the FIRST avatar/subheading/meta,
  // body collects the text roles, later meta/attribution/caption items form
  // the footer. Platform glyph keys off the generic `platform` field value —
  // additive per platform, unknown platforms simply get no glyph.
  const PLATFORM_GLYPH_PATHS: Record<string, string> = {
    twitter:
      'M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z',
  }
  const glyphPath = $derived(PLATFORM_GLYPH_PATHS[slide.values?.platform ?? ''] ?? null)
  const pc = $derived.by(() => {
    const metas = items.filter((it) => it.role === 'meta')
    return {
      avatar: items.find((it) => it.role === 'avatar'),
      name: items.find((it) => it.role === 'subheading'),
      handle: metas[0],
      body: items.filter((it) => it.role === 'heading' || it.role === 'quote' || it.role === 'body'),
      media: items.filter((it) => it.role === 'media'),
      footer: [...metas.slice(1), ...items.filter((it) => it.role === 'attribution' || it.role === 'caption')],
    }
  })

  // Auto-fit: long posts overflow at the clamp() sizes, so after each render
  // shrink the text elements step-wise until the frame fits the stage (14px
  // floor). Deliberate default, not a knob — slides size themselves.
  let stageEl = $state<HTMLElement | null>(null)
  let frameEl = $state<HTMLElement | null>(null)
  $effect(() => {
    void items
    const stage = stageEl
    const frame = frameEl
    if (!stage || !frame) return
    const els = Array.from(
      frame.querySelectorAll<HTMLElement>('.r-heading, .r-subheading, .r-body, .r-quote, .r-attribution, .r-meta, .pc-text'),
    )
    for (const el of els) el.style.fontSize = ''
    let guard = 30
    while (frame.scrollHeight > stage.clientHeight && guard-- > 0) {
      let shrunk = false
      for (const el of els) {
        const px = parseFloat(getComputedStyle(el).fontSize)
        if (px > 14) {
          el.style.fontSize = `${px * 0.92}px`
          shrunk = true
        }
      }
      if (!shrunk) break
    }
  })

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

<div class="slide-stage" style={stageStyle} bind:this={stageEl}>
  {#key slide.id}
    <div class="slide-frame {animClass}" bind:this={frameEl}>
      {#if layout === 'post-card'}
        <div class="pc">
          <div class="pc-header">
            {#if pc.avatar}
              {@const src = mediaSrc(pc.avatar.value)}
              {#if src}<img class="pc-avatar" {src} alt="" />{/if}
            {/if}
            <div class="pc-id">
              {#if pc.name}<span class="pc-name">{pc.name.value}</span>{/if}
              {#if pc.handle}<span class="pc-handle">{pc.handle.value}</span>{/if}
            </div>
            {#if glyphPath}
              <svg class="pc-glyph" viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d={glyphPath} /></svg>
            {/if}
          </div>
          {#each pc.body as item, i (i)}
            <p class="pc-text">{item.value}</p>
          {/each}
          {#each pc.media as item, i (i)}
            {@const src = mediaSrc(item.value)}
            {#if src && item.type === 'video'}
              <!-- svelte-ignore a11y_media_has_caption -->
              <video class="pc-media" {src} playsinline preload="auto" muted></video>
            {:else if src}
              <img class="pc-media" {src} alt="" />
            {/if}
          {/each}
          {#each pc.footer as item, i (i)}
            <p class="pc-footer">{item.value}</p>
          {/each}
        </div>
      {:else}
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
            <!-- Preview parity with /present: videos rest on their first
                 frame; playback is presenter-fired, never automatic. -->
            <!-- svelte-ignore a11y_media_has_caption -->
            <video class="r-media" {src} playsinline preload="auto" muted></video>
          {:else if src}
            <img class="r-media" {src} alt="" />
          {:else}
            <div class="r-placeholder">{item.value}</div>
          {/if}
        {:else if item.role === 'caption'}
          <p class="r-caption">{item.value}</p>
        {:else if item.role === 'avatar'}
          {@const src = mediaSrc(item.value)}
          {#if src}
            <img class="r-avatar" {src} alt="" />
          {/if}
        {:else if item.role === 'meta'}
          <p class="r-meta">{item.value}</p>
        {/if}
      {/each}
      {/if}
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
  .r-avatar {
    width: 4.5rem;
    height: 4.5rem;
    border-radius: 50%;
    object-fit: cover;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  }
  .r-meta {
    font-size: clamp(0.85rem, 1.4vw, 1.1rem);
    opacity: 0.55;
    margin: 0.35rem 0 0;
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

  /* post-card layout — the social-post card. Colors are the X dark-theme
     palette, deliberately hardcoded like the stage's own stage-look colors:
     slides are content with a stable look, independent of app theming. */
  .pc {
    max-width: 42rem;
    margin: 0 auto;
    background: #000;
    border: 1px solid #2f3336;
    border-radius: 16px;
    padding: 1.6rem 1.8rem;
    text-align: left;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
    color: #e7e9ea;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.55);
  }
  .pc-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  .pc-avatar {
    width: 3rem;
    height: 3rem;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
  }
  .pc-id {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .pc-name {
    font-weight: 700;
    font-size: 1.05rem;
    overflow-wrap: anywhere;
  }
  .pc-handle {
    color: #71767b;
    font-size: 0.95rem;
    overflow-wrap: anywhere;
  }
  .pc-glyph {
    width: 1.6rem;
    height: 1.6rem;
    margin-left: auto;
    flex-shrink: 0;
  }
  .pc-text {
    font-size: clamp(1.1rem, 2.2vw, 1.8rem);
    line-height: 1.4;
    margin: 1rem 0 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .pc-media {
    width: 100%;
    max-height: 60vh;
    object-fit: contain;
    border-radius: 12px;
    border: 1px solid #2f3336;
    margin-top: 1rem;
    background: #000;
  }
  .pc-footer {
    color: #71767b;
    font-size: 0.95rem;
    margin: 1rem 0 0;
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
