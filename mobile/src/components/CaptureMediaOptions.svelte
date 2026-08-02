<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { formatEstBytes, imageItems, videoItem, type VideoChoice } from '../lib/captureOptions'
  import type { CapturePreview, ImageMode } from '@shared/types'
  import RadioRows, { type RadioRow } from './RadioRows.svelte'

  // The media half of the Capture Options sheet: the video quality ladder
  // with real sizes, and what to do with a gallery.
  //
  // Every rung the platform offers is a choice the user can take —
  // including the 3.5 GB one (Harvey, 2026-08-02: "If they want to
  // download a 3.5gb video to preserve it, they should be able to").
  // BRUV pre-selects; it doesn't decide.

  let {
    preview,
    video = $bindable(),
    imageMode = $bindable(),
    disabled = false,
  }: {
    preview: CapturePreview
    video: VideoChoice | null
    imageMode: ImageMode | null
    disabled?: boolean
  } = $props()

  const item = $derived(videoItem(preview))
  const images = $derived(imageItems(preview))

  const VIDEO_LINK = 'link'
  const VIDEO_SKIP = 'skip'
  const variantKey = (id: string) => `v:${id}`

  const videoKey = $derived(
    !video ? '' : video.kind === 'variant' ? variantKey(video.id) : video.kind,
  )

  const videoRows = $derived.by(() => {
    const media = item
    if (!media) return [] as RadioRow[]
    const rows: RadioRow[] = []
    for (const variant of media.variants ?? []) {
      rows.push({ key: variantKey(variant.id), label: rungLabel(variant.label, variant.estBytes) })
    }
    // No ladder from the platform: it's still store-or-don't, just
    // without a quality to pick.
    if (!media.variants?.length) {
      rows.push({ key: variantKey(''), label: rungLabel(t('capture.video_store'), media.estBytes) })
    }
    rows.push({ key: VIDEO_LINK, label: t('capture.video_link'), sub: t('capture.video_link_sub') })
    rows.push({ key: VIDEO_SKIP, label: t('capture.video_skip') })
    return rows
  })

  function rungLabel(label: string, bytes?: number): string {
    const size = formatEstBytes(bytes)
    return size ? t('capture.variant_row', { label, size }) : label
  }

  function chooseVideo(key: string) {
    if (key === VIDEO_LINK) video = { kind: 'link' }
    else if (key === VIDEO_SKIP) video = { kind: 'skip' }
    else video = { kind: 'variant', id: key.slice(2) }
  }

  const imageRows = $derived.by(() => {
    const rows: RadioRow[] = []
    if (images.length > 1) {
      rows.push({ key: 'all', label: t('capture.images_all', { count: images.length }) })
      rows.push({ key: 'first', label: t('capture.images_first') })
    } else {
      rows.push({ key: 'all', label: t('capture.images_store') })
    }
    rows.push({ key: 'link', label: t('capture.images_link'), sub: t('capture.images_link_sub') })
    rows.push({ key: 'skip', label: t('capture.images_skip') })
    return rows
  })

  function chooseImages(key: string) {
    imageMode = key as ImageMode
  }
</script>

{#if item}
  <section class="section">
    <h3>{t('capture.video')}</h3>
    {#if item.note}
      <p class="note">{item.note}</p>
    {/if}
    <RadioRows name="capture-video" rows={videoRows} value={videoKey} onChange={chooseVideo} {disabled} />
  </section>
{/if}

{#if images.length}
  <section class="section">
    <h3>{t('capture.images')}</h3>
    <RadioRows
      name="capture-images"
      rows={imageRows}
      value={imageMode ?? 'all'}
      onChange={chooseImages}
      {disabled}
    />
  </section>
{/if}

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  h3 {
    margin: 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-faint);
  }

  .note {
    margin: 0;
    font-size: 0.78rem;
    line-height: 1.4;
    color: var(--text-muted);
  }
</style>
