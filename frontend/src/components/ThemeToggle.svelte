<script lang="ts">
  import { theme, toggleTheme } from '../lib/theme.svelte'
  import { t } from '../lib/i18n.svelte'
  import { Sun, Moon } from 'lucide-svelte'
</script>

<button class="theme-toggle" onclick={toggleTheme} title={t('theme.toggle')}>
  {#if theme.mode === 'dark'}
    <Sun size={16} />
  {:else}
    <Moon size={16} />
  {/if}
</button>

<style>
  .theme-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
    cursor: pointer;
    transition: background var(--duration-normal), color var(--duration-normal), border-color var(--duration-normal);
  }
  .theme-toggle:hover {
    background: var(--bg-subtle-hover);
    color: var(--text-primary);
    border-color: var(--border-hover);
  }
</style>
