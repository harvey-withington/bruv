/**
 * Icon value utilities.
 *
 * Entity icon fields (Brand, Stream, Project, etc) are plain strings that
 * encode one of three icon kinds, optionally wrapped with a custom colour.
 *
 * Encoding (parsed left to right):
 *
 *   ''                                    — no icon
 *   c:#rrggbb:<inner>                     — custom colour wrapper around <inner>
 *   <lucide-name>                         — curated Lucide icon
 *   data:image/png;base64,...             — uploaded raster image
 *
 * Tinting is dynamic and lives entirely in the renderer:
 *   - Lucide icons inherit `currentColor` (or the picked colour, if set).
 *   - Raster images render as a regular <img> when no colour is set, and as
 *     a CSS `mask-image` painted with the picked colour when one is set.
 *     Same source bytes either way — no baked-in alpha-only variant.
 *
 * The colour prefix is parsed by reading from offset 2 to the next `:`. Hex
 * colours never contain `:`, so this is unambiguous even when the inner value
 * is a `data:` URL.
 */

/** Canonical raster size — large enough for retina rendering at the largest
 *  display size (~22px), small enough that base64 storage stays under ~10KB. */
export const ICON_RASTER_SIZE = 64

/** Maximum accepted upload size (1 MB). Bigger files almost certainly aren't icons. */
export const MAX_UPLOAD_BYTES = 1024 * 1024

export const ACCEPTED_IMAGE_TYPES = 'image/png,image/jpeg,image/webp,image/gif,image/svg+xml'

export class ImageIconError extends Error {
  constructor(public code: 'too-large' | 'unsupported' | 'decode-failed', message: string) {
    super(message)
  }
}

/**
 * Validate and decode an image file into an HTMLImageElement.
 * The full-resolution image is kept so the editor can re-bake at any
 * scale/offset chosen by the user.
 */
export async function loadImageFromFile(file: File): Promise<HTMLImageElement> {
  if (file.size > MAX_UPLOAD_BYTES) {
    throw new ImageIconError('too-large', `File exceeds ${MAX_UPLOAD_BYTES} bytes`)
  }
  if (!file.type.startsWith('image/')) {
    throw new ImageIconError('unsupported', `Not an image: ${file.type}`)
  }
  const dataUrl = await readFileAsDataUrl(file)
  return loadImage(dataUrl)
}

/** Decode an image from a data URL or http(s) URL string. */
export async function loadImageFromUrl(src: string): Promise<HTMLImageElement> {
  return loadImage(src)
}

/**
 * Editor transform applied to an image inside a square preview frame.
 *  - `previewSize` — the on-screen square the user is positioning within.
 *  - `scale`       — multiplier on top of the contain-fit base scale.
 *                    1.0 = whole image visible (default), >1 zooms in.
 *  - `offsetX/Y`   — drag offset in preview pixels from centre.
 */
export interface IconEditorTransform {
  previewSize: number
  scale: number
  offsetX: number
  offsetY: number
}

/** Compute the base "contain-fit" scale from image natural size into preview. */
export function containScale(imgW: number, imgH: number, previewSize: number): number {
  return Math.min(previewSize / imgW, previewSize / imgH)
}

/**
 * Bake the editor view into a 64×64 PNG data URL.
 * The maths mirror the live preview's CSS transform so what the user sees in
 * the editor is exactly what gets stored.
 */
export function bakeIconFromImage(img: HTMLImageElement, transform: IconEditorTransform): string {
  const canvas = document.createElement('canvas')
  canvas.width = ICON_RASTER_SIZE
  canvas.height = ICON_RASTER_SIZE
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new ImageIconError('decode-failed', 'Canvas 2D context unavailable')

  const base = containScale(img.width, img.height, transform.previewSize)
  const displayW = img.width * base * transform.scale
  const displayH = img.height * base * transform.scale
  // Image position in preview-space, then scaled into canvas-space.
  const previewToCanvas = ICON_RASTER_SIZE / transform.previewSize
  const dx = (transform.previewSize / 2 + transform.offsetX - displayW / 2) * previewToCanvas
  const dy = (transform.previewSize / 2 + transform.offsetY - displayH / 2) * previewToCanvas
  const dw = displayW * previewToCanvas
  const dh = displayH * previewToCanvas

  ctx.clearRect(0, 0, ICON_RASTER_SIZE, ICON_RASTER_SIZE)
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(img, dx, dy, dw, dh)
  return canvas.toDataURL('image/png')
}

/**
 * Parse an icon value into its colour and inner parts.
 * The inner string is what the renderer operates on.
 */
export function parseIconValue(value: string): { color: string | null; inner: string } {
  if (value.startsWith('c:')) {
    const sep = value.indexOf(':', 2)
    if (sep > 2) {
      return { color: value.slice(2, sep), inner: value.slice(sep + 1) }
    }
  }
  return { color: null, inner: value }
}

/** Apply (or clear) a custom colour on an inner icon value. */
export function withIconColor(inner: string, color: string | null): string {
  if (!inner) return ''
  if (!color) return inner
  return `c:${color}:${inner}`
}

export function isRasterInner(inner: string): boolean {
  return inner.startsWith('data:')
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = () => reject(new ImageIconError('decode-failed', 'FileReader failed'))
    reader.readAsDataURL(file)
  })
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new ImageIconError('decode-failed', 'Image decode failed'))
    img.src = src
  })
}
