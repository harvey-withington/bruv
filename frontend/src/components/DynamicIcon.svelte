<script lang="ts">
  import { ICON_MAP } from '../lib/icons'
  import { parseIconValue, isRasterInner } from '../lib/imageIcon'

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
</style>
