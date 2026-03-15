<script lang="ts">
  import { nav } from './lib/store.svelte'
  import WelcomeScreen from './components/WelcomeScreen.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import TopBar from './components/TopBar.svelte'
  import Board from './components/Board.svelte'
  import CardDetail from './components/CardDetail.svelte'

  let searchCardId = $state<string | null>(null)

  function handleSearchSelectCard(cardId: string) {
    searchCardId = cardId
  }
</script>

{#if nav.repoOpen}
  <div class="app-shell">
    <Sidebar />
    <div class="main-area">
      <TopBar onSelectCard={handleSearchSelectCard} />
      <Board />
    </div>
  </div>

  {#if searchCardId}
    <CardDetail
      cardId={searchCardId}
      onClose={() => searchCardId = null}
    />
  {/if}
{:else}
  <WelcomeScreen />
{/if}

<style>
  .app-shell {
    display: flex;
    height: 100vh;
    overflow: hidden;
  }

  .main-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
</style>
