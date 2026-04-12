<script lang="ts">
  import { X, Plus } from 'lucide-svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'
  import { GetProfile, SetProfile } from '../lib/api'
  import { draggable } from '../lib/draggable'

  let { onClose }: { onClose: () => void } = $props()

  let profile = $state({
    display_name: '',
    role: '',
    bio: '',
    expertise: [] as string[],
    avatar_url: '',
  })
  let loaded = $state(false)
  let saved = $state(false)
  let newExpertise = $state('')

  $effect(() => {
    load()
  })

  async function load() {
    try {
      const p = await GetProfile()
      if (p) {
        profile.display_name = p.display_name || ''
        profile.role = p.role || ''
        profile.bio = p.bio || ''
        profile.expertise = p.expertise || []
        profile.avatar_url = p.avatar_url || ''
      }
    } catch { /* use defaults */ }
    loaded = true
  }

  async function save() {
    try {
      await SetProfile(profile)
      onClose()
    } catch (e) { console.error('SaveProfile:', e) }
  }

  function addExpertise() {
    const val = newExpertise.trim()
    if (!val || profile.expertise.includes(val)) { newExpertise = ''; return }
    profile.expertise = [...profile.expertise, val]
    newExpertise = ''
  }

  function removeExpertise(item: string) {
    profile.expertise = profile.expertise.filter(e => e !== item)
  }

  function handleExpertiseKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') { e.preventDefault(); addExpertise() }
  }

  function handleOverlayClick(e: MouseEvent) {
    if (e.target === e.currentTarget) onClose()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" onclick={handleOverlayClick} out:fade={{ duration: 150 }}>
  <div class="dialog" use:draggable={{ handle: '.dialog-header' }}>
    <div class="dialog-header">
      <h2>{t('profile.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    {#if loaded}
      <div class="dialog-body">
        <label class="field">
          <span class="field-label">{t('profile.display_name')}</span>
          <input type="text" bind:value={profile.display_name} placeholder={t('profile.display_name_placeholder')} />
        </label>

        <label class="field">
          <span class="field-label">{t('profile.role')}</span>
          <input type="text" bind:value={profile.role} placeholder={t('profile.role_placeholder')} />
        </label>

        <label class="field">
          <span class="field-label">{t('profile.bio')}</span>
          <textarea rows="3" bind:value={profile.bio} placeholder={t('profile.bio_placeholder')}></textarea>
        </label>

        <div class="field">
          <span class="field-label">{t('profile.expertise')}</span>
          <div class="tags-list">
            {#each profile.expertise as item}
              <span class="tag-chip">
                <span>{item}</span>
                <button class="tag-remove" onclick={() => removeExpertise(item)}><X size={10} /></button>
              </span>
            {/each}
          </div>
          <div class="add-row">
            <input
              type="text"
              bind:value={newExpertise}
              placeholder={t('profile.expertise_placeholder')}
              onkeydown={handleExpertiseKeydown}
            />
            <button class="btn-add" onclick={addExpertise}><Plus size={14} /></button>
          </div>
        </div>

      </div>

      <div class="dialog-footer">
        {#if saved}<span class="saved-msg">{t('profile.saved')}</span>{/if}
        <button class="btn btn-ghost" onclick={onClose}>{t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={save}>{t('common.save')}</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 480px;
    max-height: 85vh;
    overflow-y: auto;
    box-shadow: 0 8px 32px var(--shadow-lg);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }

  .dialog-header h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .close-btn:hover { color: var(--text-primary); }

  .dialog-body {
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .field-label {
    font-size: 0.85rem;
    font-weight: 500;
    color: var(--text-secondary);
  }

  input[type="text"], textarea {
    padding: 0.45rem 0.6rem;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
    resize: vertical;
  }
  input[type="text"]:focus, textarea:focus {
    border-color: var(--accent);
  }

  .tags-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .tag-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    background: var(--accent);
    color: #fff;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .tag-remove {
    background: none;
    border: none;
    color: rgba(255, 255, 255, 0.6);
    cursor: pointer;
    padding: 0;
    line-height: 1;
    display: flex;
    align-items: center;
  }
  .tag-remove:hover { color: #fff; }

  .add-row {
    display: flex;
    gap: 0.35rem;
  }

  .add-row input {
    flex: 1;
    min-width: 0;
  }

  .btn-add {
    background: var(--border);
    border: none;
    border-radius: 6px;
    color: var(--text-secondary);
    cursor: pointer;
    padding: 0.4rem 0.5rem;
    display: flex;
    align-items: center;
  }
  .btn-add:hover {
    background: var(--border-hover);
    color: var(--text-primary);
  }

  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--border-muted);
  }

  .saved-msg {
    font-size: 0.8rem;
    color: var(--success);
    margin-right: auto;
  }

  .btn {
    padding: 0.45rem 1rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    border: none;
  }

  .btn-primary {
    background: var(--accent);
    color: #fff;
  }
  .btn-primary:hover { background: var(--accent-hover); }

  .btn-ghost {
    background: transparent;
    color: var(--text-secondary);
  }
  .btn-ghost:hover { color: var(--text-primary); }
</style>
