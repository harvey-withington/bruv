import './style.css'
import { mount } from 'svelte'
import { initBackend } from './lib/adapters'
import App from './App.svelte'

initBackend().then(() => {
  const app = mount(App, {
    target: document.getElementById('app')!
  })
})
