import type { SlideTemplate, SlideAnimation } from './types'

// Built-in Slide Templates. A template is DATA (not code): it declares which
// content types it renders and maps each type's fields to display roles, which
// one generic renderer interprets. This keeps templates serialisable so they
// can be user-authored and shared via a registry/marketplace later (and leaves
// room for a Reveal.js-style backend). The built-ins are deliberately simple.

const CLEAN: SlideTemplate = {
  id: 'clean',
  name: 'Clean',
  supportedContentTypes: ['title', 'statement', 'lower_third'],
  fieldMap: {
    title: [
      { field: 'title', role: 'heading' },
      { field: 'subtitle', role: 'subheading' },
    ],
    statement: [{ field: 'statement', role: 'heading' }],
    lower_third: [
      { field: 'name', role: 'heading' },
      { field: 'subtitle', role: 'subheading' },
    ],
  },
  entrance: 'fadeIn',
  durationMs: 500,
}

const ELEGANT_QUOTE: SlideTemplate = {
  id: 'elegant-quote',
  name: 'Elegant Quote',
  supportedContentTypes: ['quote'],
  fieldMap: {
    quote: [
      { field: 'quote', role: 'quote' },
      { field: 'author', role: 'attribution' },
    ],
  },
  entrance: 'zoomIn',
  durationMs: 600,
}

const SHOWCASE: SlideTemplate = {
  id: 'showcase',
  name: 'Showcase',
  supportedContentTypes: ['image', 'video'],
  fieldMap: {
    image: [
      { field: 'image', role: 'media' },
      { field: 'caption', role: 'caption' },
    ],
    video: [
      { field: 'video', role: 'media' },
      { field: 'caption', role: 'caption' },
    ],
  },
  entrance: 'zoomIn',
  durationMs: 700,
}

// X_POST is the first per-platform template for the generic `post` content
// type, rendered with the 'post-card' layout: avatar + name + @handle in the
// header (first avatar/subheading/meta), post text as the body, media below,
// later meta items (date) in the footer, platform glyph from values.platform.
// `url` is deliberately unrendered (data-only). Future platforms
// (reddit-post, …) are new templates over the SAME content type — the schema
// stays generic, the template carries the platform look.
const X_POST: SlideTemplate = {
  id: 'x-post',
  name: 'X Post',
  supportedContentTypes: ['post'],
  fieldMap: {
    post: [
      { field: 'avatar', role: 'avatar' },
      { field: 'author', role: 'subheading' },
      { field: 'handle', role: 'meta' },
      { field: 'text', role: 'heading' },
      { field: 'media', role: 'media' },
      { field: 'video', role: 'media' },
      { field: 'date', role: 'meta' },
    ],
  },
  entrance: 'slideInUp',
  durationMs: 500,
  layout: 'post-card',
}

export const SLIDE_TEMPLATES: SlideTemplate[] = [CLEAN, ELEGANT_QUOTE, SHOWCASE, X_POST]

// templatesForContentType lists the templates that can render a content type
// (the editor's template picker, filtered by the slide's content type).
export function templatesForContentType(contentTypeId: string): SlideTemplate[] {
  return SLIDE_TEMPLATES.filter((t) => t.supportedContentTypes.includes(contentTypeId))
}

// resolveSlideTemplate always returns a concrete template that renders the
// given content type: the slide's own templateId if valid + compatible, else
// the first template supporting the content type, else the Clean fallback.
export function resolveSlideTemplate(templateId: string | undefined, contentTypeId: string): SlideTemplate {
  if (templateId) {
    const found = SLIDE_TEMPLATES.find((t) => t.id === templateId)
    if (found && found.supportedContentTypes.includes(contentTypeId)) return found
  }
  const supporting = templatesForContentType(contentTypeId)
  return supporting[0] ?? CLEAN
}

// entranceClass names the CSS class the renderer applies to (re-)fire a
// slide's entrance animation. Pair with a {#key slide.id} wrapper.
export function entranceClass(anim: SlideAnimation): string {
  return anim === 'none' ? '' : `slide-anim-${anim}`
}
