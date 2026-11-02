// Preparing a file for AddCardAttachment, shared by both surfaces.

/** A file's content as base64, without the data-URL prefix. */
export function fileToBase64(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      resolve(result.includes(',') ? result.split(',')[1] : result)
    }
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.readAsDataURL(file)
  })
}

/** Longest side, in pixels, of a photo taken on the phone once shrunk. */
export const PHOTO_MAX = 2048
/** Photos at or under this size and within PHOTO_MAX are sent as they are. */
const PHOTO_KEEP_BYTES = 4 << 20

/** The size that fits `w`×`h` inside `max`×`max`, never enlarging. */
export function fitSize(w: number, h: number, max = PHOTO_MAX): { w: number; h: number } {
  const k = Math.min(1, max / Math.max(w, h, 1))
  return { w: Math.round(w * k), h: Math.round(h * k) }
}

/**
 * A phone photo scaled down to PHOTO_MAX on its longest side and
 * re-encoded as JPEG, so an upload over a phone connection stays small
 * (from UNCLI). Anything that isn't a still image, is small enough
 * already, or can't be decoded is returned unchanged.
 */
export async function shrinkPhoto(file: File): Promise<File> {
  if (!file.type.startsWith('image/') || file.type === 'image/gif' || typeof createImageBitmap !== 'function') return file
  try {
    const bmp = await createImageBitmap(file)
    const { w, h } = fitSize(bmp.width, bmp.height)
    if (w === bmp.width && file.size <= PHOTO_KEEP_BYTES) {
      bmp.close()
      return file
    }
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    canvas.getContext('2d')?.drawImage(bmp, 0, 0, w, h)
    bmp.close()
    const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.85))
    if (!blob) return file
    const name = file.name.replace(/\.[^.]+$/, '') + '.jpg'
    return new File([blob], name, { type: 'image/jpeg' })
  } catch {
    return file
  }
}
