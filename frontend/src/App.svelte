<script lang="ts">
  import { Version } from '../wailsjs/go/main/App.js'

  let version = $state('...')

  $effect(() => {
    Version().then((v: string) => {
      version = v
    })
  })
</script>

<main>
  <div class="hero">
    <h1 class="logo">BRUV</h1>
    <p class="tagline">Your most organised mate.</p>
    <div class="version">v{version}</div>
  </div>
</main>

<style>
  main {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    user-select: none;
  }

  .hero {
    text-align: center;
  }

  .logo {
    font-size: 4rem;
    font-weight: 800;
    letter-spacing: 0.15em;
    margin: 0;
    color: #f5f5f5;
  }

  .tagline {
    font-size: 1.1rem;
    color: #a1a1aa;
    margin: 0.5rem 0 0;
    font-weight: 400;
  }

  .version {
    margin-top: 2rem;
    font-size: 0.75rem;
    color: #52525b;
  }
</style>
