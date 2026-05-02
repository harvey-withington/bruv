<script lang="ts" module>
  import { DotLottie } from '@lottiefiles/dotlottie-web'

  // Serve the WASM from /public so the app works offline and never reaches out
  // to a CDN — BRUV is local-first. The file is copied into public/ by the
  // `copy-vendor-assets` predev/prebuild script. Set once before any
  // DotLottie is constructed.
  DotLottie.setWasmUrl(`${import.meta.env.BASE_URL}dotlottie-player.wasm`)
</script>

<script lang="ts">
  import { onMount, onDestroy } from 'svelte'

  type Props = {
    src: string
    loop?: boolean
    autoplay?: boolean
    ariaLabel?: string
    fallback?: string
    size?: number
  }

  let {
    src,
    loop = true,
    autoplay = true,
    ariaLabel,
    fallback,
    size = 96,
  }: Props = $props()

  let canvas: HTMLCanvasElement | undefined = $state()
  let player: DotLottie | undefined
  let reduced = $state(false)

  function evaluateMotionPreference(mq: MediaQueryList) {
    reduced = mq.matches
  }

  onMount(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    evaluateMotionPreference(mq)
    const onChange = () => evaluateMotionPreference(mq)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  })

  $effect(() => {
    if (reduced || !canvas) return
    player?.destroy()
    player = new DotLottie({ canvas, src, loop, autoplay })
    return () => {
      player?.destroy()
      player = undefined
    }
  })

  onDestroy(() => {
    player?.destroy()
  })

  const fallbackText = $derived(fallback ?? ariaLabel ?? '')
</script>

{#if reduced}
  <span class="lottie-fallback">{fallbackText}</span>
{:else}
  <canvas
    bind:this={canvas}
    width={size}
    height={size}
    role={ariaLabel ? 'img' : undefined}
    aria-label={ariaLabel}
    class="lottie-canvas"
    style:width="{size}px"
    style:height="{size}px"
  ></canvas>
{/if}

<style>
  .lottie-canvas {
    display: block;
  }
  .lottie-fallback {
    color: var(--text-muted);
    font-size: 0.9rem;
  }
</style>
