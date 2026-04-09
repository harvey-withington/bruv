<script lang="ts">
  import { ICON_MAP } from '../lib/icons'

  let { name, size = 16, className = '' }: { name: string; size?: number; className?: string } = $props()

  const IconComponent = $derived(ICON_MAP[name])
</script>

{#if IconComponent}
  <span class="dynamic-icon {className}">
    <svelte:component this={IconComponent} {size} />
  </span>
{/if}

<style>
  .dynamic-icon {
    display: inline-flex;
    align-items: center;
  }
</style>
