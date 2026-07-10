<script lang="ts">
  import { X, ChevronDown, ChevronRight, Smile } from 'lucide-svelte'
  import { fade } from 'svelte/transition'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { nav, cardTypes } from '../lib/store.svelte'
  import {
    RenameProject, UpdateProjectDescription,
    ListCategories, RenameCategory, UpdateCategoryDescription, UpdateCategoryAcceptedTypes, UpdateCategoryIcon,
    PickSaveFile, ExportProjectToFile,
  } from '@shared/api'
  import { onMount, setContext } from 'svelte'
  import { EditScope, EDIT_SCOPE_KEY } from '@shared/editScope'
  import { inlineEdit } from '../lib/actions'
  import IconPicker from './IconPicker.svelte'
  import DynamicIcon from './DynamicIcon.svelte'

  let { onClose }: { onClose: () => void } = $props()

  // Keyboard entry contract: this dialog's own closable-container scope.
  // Every field here saves itself on blur/commit (no separate Save
  // button), so the container's affirmative close is just onClose.
  const editScope = new EditScope()
  editScope.requestClose = () => onClose()
  setContext(EDIT_SCOPE_KEY, editScope)

  let loaded = $state(false)
  let projectName = $state('')
  let projectDescription = $state('')
  let projectNameEl = $state<HTMLInputElement | null>(null)
  let projectDescEl = $state<HTMLTextAreaElement | null>(null)
  let categories = $state<Array<{ id: string; slug: string; name: string; description: string; icon: string; accepted_types: string[] | null }>>([])
  let expandedCat = $state<string | null>(null)
  let iconPickerCatSlug = $state<string | null>(null)
  // Row-keyed element refs for the category name/description fields —
  // used only to revert the DOM's displayed value on Escape (these
  // inputs are uncontrolled `value=` bindings, matching the existing
  // onblur-save pattern below, so cancelling doesn't touch `categories`).
  let catNameEls = $state<Record<string, HTMLInputElement | null>>({})
  let catDescEls = $state<Record<string, HTMLInputElement | null>>({})

  onMount(async () => {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    projectName = nav.projectName || ''
    projectDescription = ''

    try {
      const cats = await ListCategories(nav.brandSlug, nav.streamSlug, nav.projectSlug)
      categories = (cats || []).map((c: Record<string, unknown>) => ({
        id: c.id as string,
        slug: c.slug as string,
        name: c.name as string,
        description: (c.description as string) || '',
        icon: (c.icon as string) || '',
        accepted_types: (c.accepted_types as string[] | null) || null,
      }))
    } catch { /* ignore */ }

    loaded = true
  })

  async function saveProjectInfo() {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      if (projectName !== nav.projectName) {
        await RenameProject(nav.brandSlug, nav.streamSlug, nav.projectSlug, projectName)
        nav.projectName = projectName
      }
      if (projectDescription) {
        await UpdateProjectDescription(nav.brandSlug, nav.streamSlug, nav.projectSlug, projectDescription)
      }
      showToast(t('project_settings.saved'), 'success')
    } catch {
      showToast(t('error.save_failed'), 'error')
    }
  }

  // Plain always-visible fields (no edit-in-place toggle): Escape reverts
  // to the value loaded when the dialog opened and blurs, rather than
  // leaving a half-typed value in place.
  function cancelProjectName() { projectName = nav.projectName || ''; projectNameEl?.blur() }
  function cancelProjectDescription() { projectDescription = ''; projectDescEl?.blur() }

  async function saveCategoryField(catSlug: string, field: 'name' | 'description' | 'accepted_types' | 'icon', value: unknown) {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      if (field === 'name') {
        await RenameCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string)
      } else if (field === 'description') {
        await UpdateCategoryDescription(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string)
      } else if (field === 'accepted_types') {
        await UpdateCategoryAcceptedTypes(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string[])
      } else if (field === 'icon') {
        await UpdateCategoryIcon(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string)
      }
    } catch (e) {
      console.error(`saveCategoryField(${field}) failed:`, e)
      showToast(t('error.save_failed'), 'error')
    }
  }

  function handleIconSelected(catIdx: number, icon: string) {
    const cat = categories[catIdx]
    categories[catIdx] = { ...cat, icon }
    categories = [...categories]
    saveCategoryField(cat.slug, 'icon', icon)
    iconPickerCatSlug = null
    document.dispatchEvent(new CustomEvent('bruv:board-changed'))
  }

  let exporting = $state(false)

  async function handleExportProject() {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug || exporting) return
    exporting = true
    try {
      const defaultName = `${nav.projectSlug}-export.json`
      const path = await PickSaveFile(
        t('project_settings.export_project'),
        defaultName,
        'JSON',
        '*.json',
      )
      if (!path) {
        exporting = false
        return
      }
      await ExportProjectToFile(nav.brandSlug, nav.streamSlug, nav.projectSlug, path)
      showToast(t('project_settings.export_success', { path }), 'success')
    } catch (e) {
      console.error('ExportProjectToFile', e)
      showToast(t('project_settings.export_failed'), 'error')
    } finally {
      exporting = false
    }
  }

  function toggleCatType(catIdx: number, typeId: string) {
    const cat = categories[catIdx]
    let types = cat.accepted_types ? [...cat.accepted_types] : []
    if (types.includes(typeId)) {
      types = types.filter(t => t !== typeId)
    } else {
      types.push(typeId)
    }
    // Empty array = accept all (same as null)
    const newTypes = types.length === 0 ? null : types
    categories[catIdx] = { ...cat, accepted_types: newTypes }
    categories = [...categories]
    saveCategoryField(cat.slug, 'accepted_types', newTypes || [])
  }

  function handleCatNameBlur(catIdx: number, newName: string) {
    const cat = categories[catIdx]
    if (newName && newName !== cat.name) {
      categories[catIdx] = { ...cat, name: newName }
      categories = [...categories]
      saveCategoryField(cat.slug, 'name', newName)
    }
  }

  // Container side of the keyboard entry contract. Escape closes only
  // when nothing is being edited — field-level Escapes consume the
  // event themselves via the inlineEdit action, so this is the backstop
  // for presses that land while no field is focused.
  function handleOverlayKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (editScope.hasActive()) return
      onClose()
    } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      editScope.commitAll()
      onClose()
    }
  }

  function handleCatDescBlur(catIdx: number, newDesc: string) {
    const cat = categories[catIdx]
    if (newDesc !== cat.description) {
      categories[catIdx] = { ...cat, description: newDesc }
      categories = [...categories]
      saveCategoryField(cat.slug, 'description', newDesc)
    }
  }

  function cancelCatName(catIdx: number) {
    const cat = categories[catIdx]
    const el = catNameEls[cat.id]
    if (el) el.value = cat.name
    el?.blur()
  }

  function cancelCatDescription(catIdx: number) {
    const cat = categories[catIdx]
    const el = catDescEls[cat.id]
    if (el) el.value = cat.description
    el?.blur()
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="dialog-overlay" onclick={onClose} onkeydown={handleOverlayKeydown} out:fade={{ duration: 150 }}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="dialog" onclick={(e) => e.stopPropagation()}>
    <div class="dialog-header">
      <h2 class="dialog-title">{t('project_settings.title')}</h2>
      <button class="close-btn" onclick={onClose} title={t('common.close')}><X size={18} /></button>
    </div>

    {#if loaded}
      <div class="dialog-body">
        <!-- Project info -->
        <div class="section">
          <div class="section-label">{t('project_settings.info')}</div>
          <label class="field">
            <span class="field-label">{t('project_settings.name')}</span>
            <input
              type="text"
              bind:this={projectNameEl}
              bind:value={projectName}
              use:inlineEdit={{ onCommit: () => { void saveProjectInfo() }, onCancel: cancelProjectName }}
            />
          </label>
          <label class="field">
            <span class="field-label">{t('project_settings.description')}</span>
            <textarea
              rows="2"
              bind:this={projectDescEl}
              bind:value={projectDescription}
              placeholder={t('project_settings.description_placeholder')}
              use:inlineEdit={{ multiline: true, onCommit: () => { void saveProjectInfo() }, onCancel: cancelProjectDescription }}
            ></textarea>
          </label>
        </div>

        <!-- Categories -->
        <div class="section">
          <div class="section-label">{t('project_settings.categories')} <span class="section-count">{categories.length}</span></div>
          {#each categories as cat, i (cat.id)}
            <div class="cat-row" class:expanded={expandedCat === cat.id}>
              <div class="cat-header-row">
                <button class="cat-header" onclick={() => expandedCat = expandedCat === cat.id ? null : cat.id}>
                  <span class="cat-expand">
                    {#if expandedCat === cat.id}<ChevronDown size={14} />{:else}<ChevronRight size={14} />{/if}
                  </span>
                  <span class="cat-icon-display">
                    {#if cat.icon}
                      <DynamicIcon name={cat.icon} size={14} />
                    {/if}
                  </span>
                  <span class="cat-name">{cat.name}</span>
                  {#if cat.accepted_types?.length}
                    <span class="cat-types-badge">{t('column.n_types', { n: cat.accepted_types.length })}</span>
                  {:else}
                    <span class="cat-types-all">{t('column.all_types')}</span>
                  {/if}
                </button>
                <button
                  class="cat-icon-btn"
                  title={t('icon.pick')}
                  onclick={(e) => { e.stopPropagation(); iconPickerCatSlug = cat.slug }}
                >
                  <Smile size={13} />
                </button>
              </div>
              {#if expandedCat === cat.id}
                <div class="cat-detail">
                  <label class="field">
                    <span class="field-label">{t('project_settings.category_name')}</span>
                    <input
                      type="text"
                      value={cat.name}
                      bind:this={catNameEls[cat.id]}
                      use:inlineEdit={{
                        onCommit: () => handleCatNameBlur(i, catNameEls[cat.id]?.value ?? cat.name),
                        onCancel: () => cancelCatName(i),
                      }}
                    />
                  </label>
                  <label class="field">
                    <span class="field-label">{t('project_settings.category_description')}</span>
                    <input
                      type="text"
                      value={cat.description}
                      placeholder={t('column.descriptionPlaceholder')}
                      bind:this={catDescEls[cat.id]}
                      use:inlineEdit={{
                        onCommit: () => handleCatDescBlur(i, catDescEls[cat.id]?.value ?? cat.description),
                        onCancel: () => cancelCatDescription(i),
                      }}
                    />
                  </label>
                  <div class="field">
                    <span class="field-label">{t('project_settings.accepted_types')}</span>
                    <span class="field-hint">{t('project_settings.accepted_types_hint')}</span>
                    <div class="type-checks">
                      {#each cardTypes.list as ct (ct.id)}
                        <label class="type-check">
                          <input
                            type="checkbox"
                            checked={!cat.accepted_types || cat.accepted_types.includes(ct.id)}
                            onchange={() => toggleCatType(i, ct.id)}
                          />
                          <span class="type-dot" style="background: {ct.color}"></span>
                          {ct.label}
                        </label>
                      {/each}
                    </div>
                  </div>
                </div>
              {/if}
            </div>
          {/each}
        </div>

        <!-- Export / portability -->
        <div class="section">
          <div class="section-label">{t('project_settings.export_section')}</div>
          <p class="field-hint" style="margin: 0 0 0.5rem 0;">{t('project_settings.export_hint')}</p>
          <button class="export-btn" onclick={handleExportProject} disabled={exporting}>
            {exporting ? t('app.loading') : t('project_settings.export_project')}
          </button>
        </div>
      </div>
    {:else}
      <div class="dialog-body loading">{t('app.loading')}</div>
    {/if}
  </div>
</div>

{#if iconPickerCatSlug !== null}
  {@const idx = categories.findIndex(c => c.slug === iconPickerCatSlug)}
  {#if idx !== -1}
    <IconPicker
      value={categories[idx].icon}
      onSelect={(icon) => handleIconSelected(idx, icon)}
      onClose={() => iconPickerCatSlug = null}
    />
  {/if}
{/if}

<style>
  .dialog-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
    animation: fade-in var(--duration-normal) var(--ease-out);
  }

  .dialog {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    width: min(520px, 90vw);
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 12px 40px rgba(0,0,0,0.25);
    animation: fade-in-scale var(--duration-moderate) var(--ease-out);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }
  .dialog-title {
    font-size: 0.95rem;
    font-weight: 600;
    margin: 0;
    flex: 1;
  }
  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: 4px;
  }
  .close-btn:hover { color: var(--text-primary); }

  .dialog-body {
    overflow-y: auto;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }
  .dialog-body.loading {
    padding: 2rem;
    text-align: center;
    color: var(--text-muted);
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .section-label {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }
  .section-count {
    font-size: 0.6rem;
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    padding: 0 0.3rem;
    border-radius: 3px;
    color: var(--text-muted);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .field-label {
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--text-secondary);
  }
  .field-hint {
    font-size: 0.7rem;
    color: var(--text-muted);
  }

  input, textarea {
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 0.85rem;
    font-family: inherit;
    outline: none;
    resize: vertical;
  }
  input:focus, textarea:focus { border-color: var(--accent); }

  /* Categories */
  .cat-row {
    border: 1px solid var(--border-muted);
    border-radius: 6px;
    overflow: hidden;
    transition: border-color 0.15s;
  }
  .cat-row.expanded {
    border-color: color-mix(in srgb, var(--accent) 40%, var(--border));
  }

  .cat-header-row {
    display: flex;
    align-items: stretch;
  }
  .cat-header {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.45rem 0.6rem;
    flex: 1;
    min-width: 0;
    background: none;
    border: none;
    color: var(--text-body);
    cursor: pointer;
    font-size: 0.82rem;
    text-align: left;
    transition: background 0.1s;
  }
  .cat-header:hover { background: var(--bg-hover); }
  .cat-icon-display {
    display: inline-flex;
    align-items: center;
    color: var(--text-muted);
    flex-shrink: 0;
  }
  .cat-icon-display:empty { display: none; }
  .cat-icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0 0.55rem;
    background: none;
    border: none;
    border-left: 1px solid var(--border-muted);
    color: var(--text-muted);
    cursor: pointer;
    transition: color 0.1s, background 0.1s;
  }
  .cat-icon-btn:hover {
    color: var(--icon-picker-accent);
    background: var(--bg-hover);
  }

  .cat-expand { color: var(--text-muted); display: flex; }
  .cat-name { font-weight: 500; flex: 1; }
  .cat-types-badge {
    font-size: 0.65rem;
    padding: 0.05rem 0.3rem;
    border-radius: 3px;
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    color: var(--text-muted);
  }
  .cat-types-all {
    font-size: 0.65rem;
    color: var(--text-muted);
    font-style: italic;
  }

  .cat-detail {
    padding: 0.65rem;
    border-top: 1px solid var(--border-muted);
    background: var(--bg-elevated);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .type-checks {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin-top: 0.2rem;
  }
  .type-check {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    font-size: 0.78rem;
    cursor: pointer;
    color: var(--text-body);
  }
  .type-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .export-btn {
    padding: 0.4rem 0.9rem;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-body);
    font-size: 0.85rem;
    font-family: inherit;
    cursor: pointer;
    align-self: flex-start;
  }
  .export-btn:hover:not(:disabled) {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
  }
  .export-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
