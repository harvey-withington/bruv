<script lang="ts">
  import type { Slide, DeckTheme, SlideDisplayRole, TemplatePrefs } from '@shared/types'
  import { resolveSlideTemplate, entranceClass } from '@shared/slideTemplates'
  import { resolveContentType } from '@shared/slideContentTypes'

  // Generic, data-driven renderer: it reads the template's field→role map for
  // the slide's content type and renders each field by its display role.
  // Field values come from slide.values, or from a live binding the caller
  // resolves (desktop store / server-side for /present) — injected so this
  // component stays surface-agnostic. Media values resolve to a URL likewise.
  // templatePrefs (vault Auto-matching prefs) are injected the same way.
  let {
    slide,
    deckTheme,
    resolveField,
    resolveMediaUrl,
    templatePrefs,
    onOverflowChange,
  }: {
    slide: Slide
    deckTheme?: DeckTheme
    resolveField?: (slide: Slide, fieldKey: string) => string | undefined
    resolveMediaUrl?: (value: string) => string | undefined
    templatePrefs?: TemplatePrefs
    // Fires when the slide's content exceeds the stage (scale backstop
    // engaged, or scrollable in scroll mode) — editor shows a badge.
    onOverflowChange?: (overflowing: boolean) => void
  } = $props()

  const contentType = $derived(resolveContentType(slide.contentTypeId))
  // Auto resolution needs the capture URL post-binding-resolution — clipped
  // slides bind `url` to the card's Source block rather than storing it.
  const template = $derived(
    resolveSlideTemplate(slide.templateId, slide.contentTypeId, fieldValue('url') || undefined, templatePrefs),
  )
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

  // Multi-URL media values (newline-joined — a gallery) render as a
  // carousel: one image at a time, click advances with wrap, counter badge.
  // On /present the console's Images button advances via carouselSeq.
  function mediaUrls(value: string): string[] {
    return value.split('\n').filter(Boolean)
  }
  let carouselIdx = $state(0)
  $effect(() => {
    void slide.id
    carouselIdx = 0
  })
  function advanceCarousel(count: number): void {
    carouselIdx = (carouselIdx + 1) % count
  }

  // --- post-card layout ---
  // Partition items by role: header takes the FIRST avatar/subheading/meta,
  // body collects the text roles, later meta/attribution/caption items form
  // the footer. Platform glyph keys off the generic `platform` field value —
  // additive per platform, unknown platforms simply get no glyph.
  // twitter/reddit/youtube are Simple Icons brand paths; truthsocial is a
  // hand-drawn "T." wordmark (not in Simple Icons).
  const PLATFORM_GLYPH_PATHS: Record<string, string> = {
    twitter:
      'M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z',
    truthsocial: 'M3 4h13v4h-4.5v12H7V8H3V4zm14.5 12H21v4h-3.5v-4z',
    reddit:
      'M12 0C5.373 0 0 5.373 0 12c0 3.314 1.343 6.314 3.515 8.485l-2.286 2.286C.775 23.225 1.097 24 1.738 24H12c6.627 0 12-5.373 12-12S18.627 0 12 0Zm4.388 3.199c1.104 0 1.999.895 1.999 1.999 0 1.105-.895 2-1.999 2-.946 0-1.739-.657-1.947-1.539v.002c-1.147.162-2.032 1.15-2.032 2.341v.007c1.776.067 3.4.567 4.686 1.363.473-.363 1.064-.58 1.707-.58 1.547 0 2.802 1.254 2.802 2.802 0 1.117-.655 2.081-1.601 2.531-.088 3.256-3.637 5.876-7.997 5.876-4.361 0-7.905-2.617-7.998-5.87-.954-.447-1.614-1.415-1.614-2.538 0-1.548 1.255-2.802 2.803-2.802.645 0 1.239.218 1.712.585 1.275-.79 2.881-1.291 4.64-1.365v-.01c0-1.663 1.263-3.034 2.88-3.207.188-.911.993-1.595 1.959-1.595Zm-8.085 8.376c-.784 0-1.459.78-1.506 1.797-.047 1.016.64 1.429 1.426 1.429.786 0 1.371-.369 1.418-1.385.047-1.017-.553-1.841-1.338-1.841Zm7.406 0c-.786 0-1.385.824-1.338 1.841.047 1.017.634 1.385 1.418 1.385.785 0 1.473-.413 1.426-1.429-.046-1.017-.721-1.797-1.506-1.797Zm-3.703 4.013c-.974 0-1.907.048-2.77.135-.147.015-.241.168-.183.305.483 1.154 1.622 1.964 2.953 1.964 1.33 0 2.47-.81 2.953-1.964.057-.137-.037-.29-.184-.305-.863-.087-1.795-.135-2.769-.135Z',
    youtube:
      'M23.498 6.186a3.016 3.016 0 0 0-2.122-2.136C19.505 3.545 12 3.545 12 3.545s-7.505 0-9.377.505A3.017 3.017 0 0 0 .502 6.186C0 8.07 0 12 0 12s0 3.93.502 5.814a3.016 3.016 0 0 0 2.122 2.136c1.871.505 9.376.505 9.376.505s7.505 0 9.377-.505a3.015 3.015 0 0 0 2.122-2.136C24 15.93 24 12 24 12s0-3.93-.502-5.814zM9.545 15.568V8.432L15.818 12l-6.273 3.568z',
  }

  // "embed://<provider>/<id>" media values render the platform's official
  // iframe player (YouTube: no downloadable stream exists — see the video
  // capture modes section of the 2026-07-31 template plan). Allowlist-parsed;
  // unknown providers render nothing.
  function embedSrc(value: string): string | undefined {
    const m = value.match(/^embed:\/\/([a-z0-9-]+)\/([A-Za-z0-9_-]+)$/)
    if (!m) return undefined
    // www.youtube.com, NOT youtube-nocookie.com: the no-cookie domain sends
    // no session at all, which trips YouTube's "sign in to confirm you're
    // not a bot" wall. The standard domain rides the viewer's own YouTube
    // login (a personal tool presenting in the user's own browser).
    if (m[1] === 'youtube') return `https://www.youtube.com/embed/${m[2]}?rel=0&enablejsapi=1`
    return undefined
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

  // Overflow handling (per-slide `overflow` option, default 'fit'):
  //  - fit:    shrink text step-wise to a 14px floor, then transform-scale
  //            the whole fit wrapper as the backstop — a slide can NEVER
  //            clip. The scale lives on the inner wrapper, not the frame:
  //            entrance animations animate the frame's transform and would
  //            fight an inline scale there.
  //  - scroll: render full-size; the stage scrolls (themed scrollbar on
  //            desktop; console-paged on /present).
  const scrollMode = $derived(slide.overflow === 'scroll')
  let stageEl = $state<HTMLElement | null>(null)
  let fitEl = $state<HTMLElement | null>(null)
  $effect(() => {
    void items
    void carouselIdx // gallery images differ in height — refit per advance
    const stage = stageEl
    const fit = fitEl
    if (!stage || !fit) return

    const runFit = (): void => {
      const els = Array.from(
        fit.querySelectorAll<HTMLElement>('.r-heading, .r-subheading, .r-body, .r-quote, .r-attribution, .r-meta, .pc-text'),
      )
      for (const el of els) el.style.fontSize = ''
      fit.style.transform = ''
      // Available height = the stage's content box (clientHeight includes
      // the stage padding).
      const cs = getComputedStyle(stage)
      const avail = stage.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom)
      if (scrollMode) {
        onOverflowChange?.(fit.scrollHeight > avail)
        return
      }
      let guard = 30
      while (fit.scrollHeight > avail && guard-- > 0) {
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
      // Scale backstop: whatever still doesn't fit is shrunk proportionally
      // (media included). Scaled around center — the stage centers the
      // frame, so the visible region is exactly the scaled content.
      const overflowing = fit.scrollHeight > avail
      if (overflowing) {
        fit.style.transform = `scale(${avail / fit.scrollHeight})`
      }
      onOverflowChange?.(overflowing)
    }

    runFit()
    // Media sizes arrive AFTER the first pass (imgs/videos have no height
    // on fresh DOM) — refit as each one reports in, or the slides that most
    // need the backstop are the ones that skip it.
    const pendingImgs = Array.from(fit.querySelectorAll('img')).filter((i) => !i.complete)
    const vids = Array.from(fit.querySelectorAll('video'))
    for (const i of pendingImgs) i.addEventListener('load', runFit, { once: true })
    for (const v of vids) v.addEventListener('loadedmetadata', runFit, { once: true })
    return () => {
      for (const i of pendingImgs) i.removeEventListener('load', runFit)
      for (const v of vids) v.removeEventListener('loadedmetadata', runFit)
    }
  })

  // Slides are content, not app chrome — stable dark-stage look regardless of
  // the app theme, overridable per deck/template via scoped CSS custom props.
  // The --pc-* tokens are the post-card platform skin (template style data).
  const stageStyle = $derived(
    [
      `--slide-bg:${deckTheme?.transparent ? 'transparent' : deckTheme?.backgroundColor ?? template.styles?.backgroundColor ?? '#0b0b12'}`,
      `--slide-fg:${deckTheme?.textColor ?? template.styles?.textColor ?? '#ffffff'}`,
      `--slide-accent:${deckTheme?.accentColor ?? template.styles?.accentColor ?? '#8b5cf6'}`,
      `--slide-anim-ms:${template.durationMs}ms`,
      deckTheme?.fontFamily ? `--slide-font:${deckTheme.fontFamily}` : '',
      template.styles?.cardBackgroundColor ? `--pc-bg:${template.styles.cardBackgroundColor}` : '',
      template.styles?.cardBorderColor ? `--pc-border:${template.styles.cardBorderColor}` : '',
      template.styles?.cardTextColor ? `--pc-fg:${template.styles.cardTextColor}` : '',
      template.styles?.cardMutedColor ? `--pc-muted:${template.styles.cardMutedColor}` : '',
      template.styles?.cardAccentColor ? `--pc-accent:${template.styles.cardAccentColor}` : '',
      template.styles?.cardFontFamily ? `--pc-font:${template.styles.cardFontFamily}` : '',
    ]
      .filter(Boolean)
      .join(';'),
  )
</script>

<div class="slide-stage" class:scrolling={scrollMode} style={stageStyle} bind:this={stageEl}>
  {#key slide.id}
    <div class="slide-frame {animClass}">
      <div class="frame-fit" bind:this={fitEl}>
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
            {@const embed = embedSrc(item.value)}
            {@const urls = embed ? [] : mediaUrls(item.value)}
            {#if embed}
              <iframe class="pc-embed" src={embed} title="Embedded video" allow="autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>
            {:else if urls.length > 1 && item.type !== 'video'}
              {@const idx = carouselIdx % urls.length}
              {@const src = mediaSrc(urls[idx])}
              <button class="carousel" type="button" onclick={() => advanceCarousel(urls.length)}>
                {#if src}<img class="pc-media" {src} alt="" />{/if}
                <span class="car-count">{idx + 1}/{urls.length}</span>
              </button>
            {:else}
              {@const src = mediaSrc(urls[0] ?? item.value)}
              {#if src && item.type === 'video'}
                <!-- svelte-ignore a11y_media_has_caption -->
                <video class="pc-media" {src} playsinline preload="auto" muted></video>
              {:else if src}
                <img class="pc-media" {src} alt="" />
              {/if}
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
          {@const embed = embedSrc(item.value)}
          {@const urls = embed ? [] : mediaUrls(item.value)}
          {#if embed}
            <iframe class="r-embed" src={embed} title="Embedded video" allow="autoplay; encrypted-media; picture-in-picture" allowfullscreen></iframe>
          {:else if urls.length > 1 && item.type !== 'video'}
            {@const idx = carouselIdx % urls.length}
            {@const src = mediaSrc(urls[idx])}
            <button class="carousel" type="button" onclick={() => advanceCarousel(urls.length)}>
              {#if src}<img class="r-media" {src} alt="" />{/if}
              <span class="car-count">{idx + 1}/{urls.length}</span>
            </button>
          {:else}
            {@const src = mediaSrc(urls[0] ?? item.value)}
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
  /* Scale-backstop host: transform lives here, NOT on .slide-frame — the
     entrance keyframes animate the frame's transform and the two would
     fight (visible jump when the animation's final frame releases). */
  .frame-fit {
    width: 100%;
  }
  /* Scroll mode: full-size content, stage scrolls. Desktop/editor get a
     themed scrollbar; /present pages via the console instead. */
  .slide-stage.scrolling {
    align-items: flex-start;
    overflow-y: auto;
    scrollbar-width: thin;
    scrollbar-color: color-mix(in srgb, var(--slide-fg) 30%, transparent) transparent;
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

  /* post-card layout — the social-post card. Neutral dark defaults,
     deliberately independent of app theming (slides are content with a
     stable look); each template's style tokens override the --pc-* vars
     via stageStyle (the platform skin: X black, Reddit dark, YouTube…). */
  .pc {
    max-width: 42rem;
    margin: 0 auto;
    background: var(--pc-bg, #000);
    border: 1px solid var(--pc-border, #2f3336);
    border-radius: 16px;
    padding: 1.6rem 1.8rem;
    text-align: left;
    font-family: var(--pc-font, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif);
    color: var(--pc-fg, #e7e9ea);
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
    color: var(--pc-muted, #71767b);
    font-size: 0.95rem;
    overflow-wrap: anywhere;
  }
  .pc-glyph {
    width: 1.6rem;
    height: 1.6rem;
    margin-left: auto;
    flex-shrink: 0;
    color: var(--pc-accent, currentColor);
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
    border: 1px solid var(--pc-border, #2f3336);
    margin-top: 1rem;
    background: #000;
  }
  .pc-embed {
    width: 100%;
    aspect-ratio: 16 / 9;
    border: 1px solid var(--pc-border, #2f3336);
    border-radius: 12px;
    margin-top: 1rem;
    background: #000;
  }
  .pc-footer {
    color: var(--pc-muted, #71767b);
    font-size: 0.95rem;
    margin: 1rem 0 0;
  }
  .r-embed {
    width: 100%;
    aspect-ratio: 16 / 9;
    border: 0;
    border-radius: 8px;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
  }
  /* Gallery carousel: the whole image is the advance button (no chrome —
     BRUV's "everything you expect to be clickable is"), counter badge in
     the corner. */
  .carousel {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    border: 0;
    background: none;
    cursor: pointer;
    font: inherit;
    color: inherit;
  }
  .car-count {
    position: absolute;
    top: 0.6rem;
    right: 0.6rem;
    padding: 2px 9px;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.65);
    color: #fff;
    font-size: 0.75rem;
    font-weight: 600;
    pointer-events: none;
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
