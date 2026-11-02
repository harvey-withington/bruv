import { describe, it, expect } from 'vitest'
import { fileToBase64, fitSize, shrinkPhoto, PHOTO_MAX } from './attachmentUpload'

describe('fitSize', () => {
  it('scales the longest side down to the limit, keeping the ratio', () => {
    expect(fitSize(4032, 3024)).toEqual({ w: PHOTO_MAX, h: 1536 })
    expect(fitSize(3024, 4032)).toEqual({ w: 1536, h: PHOTO_MAX })
  })

  it('never enlarges', () => {
    expect(fitSize(800, 600)).toEqual({ w: 800, h: 600 })
  })
})

describe('shrinkPhoto', () => {
  it('leaves files that are not still images alone', async () => {
    const pdf = new File(['%PDF'], 'spec.pdf', { type: 'application/pdf' })
    const gif = new File(['GIF89a'], 'anim.gif', { type: 'image/gif' })
    expect(await shrinkPhoto(pdf)).toBe(pdf)
    expect(await shrinkPhoto(gif)).toBe(gif)
  })
})

describe('fileToBase64', () => {
  it('returns the content without the data-URL prefix', async () => {
    expect(await fileToBase64(new Blob(['hi'], { type: 'text/plain' }))).toBe('aGk=')
  })
})
