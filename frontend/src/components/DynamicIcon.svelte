<script lang="ts">
  import { ICON_MAP } from '@shared/icons'
  import { parseIconValue, isRasterInner } from '@shared/imageIcon'
  import { t } from '../lib/i18n.svelte'

  let { name, size = 16, className = '' }: { name: string; size?: number; className?: string } = $props()

  const parsed = $derived(parseIconValue(name))
  const inner = $derived(parsed.inner)
  const color = $derived(parsed.color)

  const isRaster = $derived(isRasterInner(inner))
  const IconComponent = $derived(isRaster ? null : ICON_MAP[inner])

  // For Lucide icons, applying `color:` lets the SVG pick it up via currentColor.
  const colorStyle = $derived(color ? `color:${color};` : '')
</script>

{#if isRaster}
  {#if color}
    <!-- Tinted raster: paint the picked colour through the alpha channel via mask-image. -->
    <span
      class="dynamic-icon raster-tinted {className}"
      style="--icon-size:{size}px; --icon-mask:url('{inner}'); {colorStyle}"
      aria-hidden="true"
    ></span>
  {:else}
    <!-- Untinted raster: render with original colours. -->
    <span class="dynamic-icon raster {className}" style="--icon-size:{size}px;">
      <img src={inner} alt="" width={size} height={size} draggable="false" />
    </span>
  {/if}
{:else if IconComponent}
  <span class="dynamic-icon {className}" style={colorStyle}>
    <IconComponent {size} />
  </span>
{:else if inner}
  <!-- Unknown icon name — render a visible placeholder so the symptom isn't
       silent. The full name is in the tooltip so the user can report it and
       the icon can be added to ICON_MAP. -->
  <span
    class="dynamic-icon dynamic-icon-unknown {className}"
    style="--icon-size:{size}px; {colorStyle}"
    title={t('icon.unknown', { name: inner })}
    aria-label={t('icon.unknown', { name: inner })}
  >?</span>
{/if}

<style>
  .dynamic-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .raster {
    width: var(--icon-size);
    height: var(--icon-size);
  }
  .raster img {
    display: block;
    object-fit: contain;
  }
  .raster-tinted {
    width: var(--icon-size);
    height: var(--icon-size);
    background-color: currentColor;
    -webkit-mask-image: var(--icon-mask);
    mask-image: var(--icon-mask);
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: center;
    mask-position: center;
    -webkit-mask-size: contain;
    mask-size: contain;
  }
  .dynamic-icon-unknown {
    width: var(--icon-size);
    height: var(--icon-size);
    border: 1px dashed currentColor;
    border-radius: 3px;
    font-size: calc(var(--icon-size) * 0.7);
    line-height: 1;
    font-weight: 700;
    opacity: 0.6;
    cursor: help;
  }
</style>
