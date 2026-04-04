<script lang="ts">
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { showConfirm } from '../lib/confirm.svelte'
  import { Plus, Pencil, Trash2, X } from 'lucide-svelte'
  import CardTypeEditor from './CardTypeEditor.svelte'
  import {
    CreateUserCardType, UpdateUserCardType, DeleteUserCardType,
    ListCardTemplates, CreateCardTemplate, UpdateCardTemplate, DeleteCardTemplate,
  } from '../lib/api'
  import { cardTypes, loadCardTypes } from '../lib/store.svelte'
  import { draggable } from '../lib/draggable'
  import { focusTrap, portal } from '../lib/actions'
  import type { CardTypeInfo, UserCardType, CardTemplate } from '../lib/types'

  let { onClose }: { onClose: () => void } = $props()

  // Built-in types come from the store; user types are the non-builtin ones
  let builtinTypes = $derived(cardTypes.list.filter(t => t.builtin))
  let userTypes = $derived(cardTypes.list.filter(t => !t.builtin))

  let templates = $state<CardTemplate[]>([])
  let loadingTemplates = $state(false)

  async function loadTemplates() {
    loadingTemplates = true
    try {
      templates = await ListCardTemplates() || []
    } catch {
      templates = []
    }
    loadingTemplates = false
  }

  $effect(() => { loadTemplates() })

  // Editor modal state
  let editingType = $state<CardTypeInfo | undefined>(undefined)
  let showEditor = $state(false)

  function openCreate() {
    editingType = undefined
    showEditor = true
  }

  function openEdit(type: CardTypeInfo) {
    editingType = type
    showEditor = true
  }

  async function handleSave(saved: UserCardType, updatedTemplates: CardTemplate[]) {
    try {
      // Sync any new or updated templates
      const templateIdMap: Record<string, string> = {}
      for (const tmpl of updatedTemplates) {
        if (tmpl.id.startsWith('__new__')) {
          const created = await CreateCardTemplate(tmpl.name, tmpl.blocks)
          templateIdMap[tmpl.id] = created.id
        } else if (templates.some(t => t.id === tmpl.id)) {
          await UpdateCardTemplate(tmpl.id, tmpl.name, tmpl.blocks)
          templateIdMap[tmpl.id] = tmpl.id
        }
      }

      // Resolve the template_id if it was a temp id
      const resolvedTemplateId = saved.template_id
        ? (templateIdMap[saved.template_id] ?? saved.template_id)
        : ''

      if (editingType) {
        await UpdateUserCardType(
          editingType.id,
          saved.label,
          saved.color,
          saved.description,
          saved.ai_hint ?? '',
          resolvedTemplateId,
        )
      } else {
        await CreateUserCardType(
          saved.label,
          saved.color,
          saved.description,
          saved.ai_hint ?? '',
          resolvedTemplateId,
        )
      }

      await loadCardTypes()
      await loadTemplates()
      showEditor = false
    } catch (e) {
      showToast(t('error.card_type_save_failed'), 'error')
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !showEditor) onClose()
  }

  function handleOverlayClick(e: MouseEvent) {
    if (showEditor) return
    if (e.target === e.currentTarget) onClose()
  }

  async function handleDelete(type: CardTypeInfo) {
    const ok = await showConfirm(
      t('card_types.confirm_delete').replace('{label}', type.label)
    )
    if (!ok) return
    try {
      await DeleteUserCardType(type.id)
      await loadCardTypes()
    } catch {
      showToast(t('error.card_type_delete_failed'), 'error')
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" role="presentation" use:portal onclick={handleOverlayClick}>
  <div class="dialog" role="dialog" use:draggable={{ handle: '.dialog-header' }} use:focusTrap>
    <div class="dialog-header">
      <h2>{t('card_types.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    <div class="dialog-body">
      <section class="section">
        <div class="section-header">
          <h3 class="section-title">{t('card_types.section_user')}</h3>
          <button class="add-btn" onclick={openCreate}>
            <Plus size={13} /> {t('card_types.add')}
          </button>
        </div>

        {#if userTypes.length === 0}
          <p class="empty-hint">{t('card_types.empty')}</p>
        {:else}
          <ul class="type-list">
            {#each userTypes as type (type.id)}
              <li class="type-row">
                <span class="type-swatch" style:background={type.color}></span>
                <div class="type-info">
                  <span class="type-label">{type.label}</span>
                  {#if type.description}
                    <span class="type-desc">{type.description}</span>
                  {/if}
                </div>
                <div class="type-actions">
                  <button class="icon-btn" onclick={() => openEdit(type)} title={t('card_types.edit')}>
                    <Pencil size={13} />
                  </button>
                  <button class="icon-btn danger" onclick={() => handleDelete(type)} title={t('card_types.delete')}>
                    <Trash2 size={13} />
                  </button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="section">
        <div class="section-header">
          <h3 class="section-title">{t('card_types.section_builtin')}</h3>
        </div>
        <ul class="type-list readonly">
          {#each builtinTypes as type (type.id)}
            <li class="type-row">
              <span class="type-swatch" style:background={type.color}></span>
              <div class="type-info">
                <span class="type-label">{type.label}</span>
                {#if type.description}
                  <span class="type-desc">{type.description}</span>
                {/if}
              </div>
              <span class="builtin-badge">{t('card_types.builtin_badge')}</span>
            </li>
          {/each}
        </ul>
      </section>
    </div>
  </div>

  {#if showEditor}
    <CardTypeEditor
      type={editingType}
      {templates}
      allTypes={cardTypes.list}
      onSave={handleSave}
      onClose={() => showEditor = false}
    />
  {/if}
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
  }
  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: 480px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px var(--shadow-lg);
  }
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--border-muted);
  }
  .dialog-header h2 { font-size: 1.1rem; font-weight: 600; margin: 0; }
  .close-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 0.25rem;
    line-height: 1;
    border-radius: 4px;
  }
  .close-btn:hover { color: var(--text-primary); }
  .dialog-body {
    padding: 1.25rem;
    overflow-y: auto;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 24px;
    min-height: 0;
  }

  .section { display: flex; flex-direction: column; gap: 10px; }
  .section-header { display: flex; align-items: center; justify-content: space-between; }
  .section-title { font-size: 13px; font-weight: 600; margin: 0; color: var(--text); }

  .add-btn {
    display: flex;
    align-items: center;
    gap: 5px;
    background: none;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 5px 10px;
    font-size: 12px;
    color: var(--text-muted);
    cursor: pointer;
  }
  .add-btn:hover { color: var(--text); border-color: var(--text-muted); }

  .empty-hint { font-size: 12px; color: var(--text-muted); margin: 0; padding: 8px 0; }

  .type-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }

  .type-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  .type-swatch { width: 12px; height: 12px; border-radius: 50%; flex-shrink: 0; }

  .type-info { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .type-label { font-size: 13px; font-weight: 500; }
  .type-desc { font-size: 11px; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  .type-actions { display: flex; gap: 4px; }
  .icon-btn {
    background: none;
    border: none;
    cursor: pointer;
    color: var(--text-muted);
    padding: 3px;
    border-radius: 4px;
    line-height: 1;
  }
  .icon-btn:hover { color: var(--text); background: var(--bg-hover); }
  .icon-btn.danger:hover { color: var(--danger); }

  .builtin-badge {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    padding: 1px 6px;
    border-radius: 4px;
    flex-shrink: 0;
  }
</style>
