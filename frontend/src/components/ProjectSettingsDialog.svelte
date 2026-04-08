<script lang="ts">
  import { X, ChevronDown, ChevronRight } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'
  import { showToast } from '../lib/toast.svelte'
  import { nav, cardTypes } from '../lib/store.svelte'
  import {
    RenameProject, UpdateProjectDescription,
    ListCategories, RenameCategory, UpdateCategoryDescription, UpdateCategoryAcceptedTypes,
  } from '../lib/api'
  import { onMount } from 'svelte'

  let { onClose }: { onClose: () => void } = $props()

  let loaded = $state(false)
  let projectName = $state('')
  let projectDescription = $state('')
  let categories = $state<Array<{ id: string; slug: string; name: string; description: string; accepted_types: string[] | null }>>([])
  let expandedCat = $state<string | null>(null)

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

  async function saveCategoryField(catSlug: string, field: 'name' | 'description' | 'accepted_types', value: unknown) {
    if (!nav.brandSlug || !nav.streamSlug || !nav.projectSlug) return
    try {
      if (field === 'name') {
        await RenameCategory(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string)
      } else if (field === 'description') {
        await UpdateCategoryDescription(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string)
      } else if (field === 'accepted_types') {
        await UpdateCategoryAcceptedTypes(nav.brandSlug, nav.streamSlug, nav.projectSlug, catSlug, value as string[])
      }
    } catch {
      showToast(t('error.save_failed'), 'error')
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

  function handleCatDescBlur(catIdx: number, newDesc: string) {
    const cat = categories[catIdx]
    if (newDesc !== cat.description) {
      categories[catIdx] = { ...cat, description: newDesc }
      categories = [...categories]
      saveCategoryField(cat.slug, 'description', newDesc)
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="dialog-overlay" onclick={onClose} onkeydown={(e) => { if (e.key === 'Escape') onClose() }}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
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
              bind:value={projectName}

              onblur={saveProjectInfo}
            />
          </label>
          <label class="field">
            <span class="field-label">{t('project_settings.description')}</span>
            <textarea
              rows="2"
              bind:value={projectDescription}
              placeholder={t('project_settings.description_placeholder')}

              onblur={saveProjectInfo}
            ></textarea>
          </label>
        </div>

        <!-- Categories -->
        <div class="section">
          <div class="section-label">{t('project_settings.categories')} <span class="section-count">{categories.length}</span></div>
          {#each categories as cat, i (cat.id)}
            <div class="cat-row" class:expanded={expandedCat === cat.id}>
              <button class="cat-header" onclick={() => expandedCat = expandedCat === cat.id ? null : cat.id}>
                <span class="cat-expand">
                  {#if expandedCat === cat.id}<ChevronDown size={14} />{:else}<ChevronRight size={14} />{/if}
                </span>
                <span class="cat-name">{cat.name}</span>
                {#if cat.accepted_types?.length}
                  <span class="cat-types-badge">{cat.accepted_types.length} types</span>
                {:else}
                  <span class="cat-types-all">{t('column.all_types')}</span>
                {/if}
              </button>
              {#if expandedCat === cat.id}
                <div class="cat-detail">
                  <label class="field">
                    <span class="field-label">{t('project_settings.category_name')}</span>
                    <input
                      type="text"
                      value={cat.name}
                      onblur={(e) => handleCatNameBlur(i, (e.target as HTMLInputElement).value)}
                    />
                  </label>
                  <label class="field">
                    <span class="field-label">{t('project_settings.category_description')}</span>
                    <input
                      type="text"
                      value={cat.description}
                      placeholder={t('column.descriptionPlaceholder')}
                      onblur={(e) => handleCatDescBlur(i, (e.target as HTMLInputElement).value)}
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
      </div>
    {:else}
      <div class="dialog-body loading">{t('app.loading')}</div>
    {/if}
  </div>
</div>

<style>
  .dialog-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 200;
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

  .cat-header {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.45rem 0.6rem;
    width: 100%;
    background: none;
    border: none;
    color: var(--text-body);
    cursor: pointer;
    font-size: 0.82rem;
    text-align: left;
    transition: background 0.1s;
  }
  .cat-header:hover { background: var(--bg-hover); }

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
</style>
