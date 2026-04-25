<script lang="ts">
  // First-run enrolment for browser-mode clients (no Wails Shell, no
  // saved credentials in localStorage). Shown by main.ts when the
  // cloud adapter throws NeedsEnrolmentError.
  //
  // Desktop-mode users never see this screen — they always have an
  // implicit Local connection and manage remotes via the Connections
  // dialog (see ConnectionsDialog.svelte).
  //
  // The form itself is shared with the desktop-mode connections dialog;
  // the only difference is the persistence callback. Browser mode
  // writes to localStorage via saveEnrolment, then reloads so the
  // cloud adapter picks up the new credentials.

  import { t } from '../lib/i18n.svelte'
  import { saveEnrolment } from '../lib/adapters/cloud'
  import AddConnectionForm from './AddConnectionForm.svelte'

  function handleEnrolled(args: { name: string; url: string; deviceToken: string }) {
    saveEnrolment(args.url, args.deviceToken)
    // Reload so the freshly-saved credentials take effect through
    // the normal cloud-adapter bootstrap path. Simpler than trying
    // to re-run initBackend in place.
    window.location.reload()
  }
</script>

<div class="screen">
  <div class="card">
    <h1>{t('enrol.title')}</h1>
    <p class="subtitle">{t('enrol.subtitle')}</p>
    <AddConnectionForm
      initialURL={typeof window !== 'undefined' ? window.location.origin : ''}
      onEnrolled={handleEnrolled}
      submitLabel={t('connection.submit')}
    />
  </div>
</div>

<style>
  .screen {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    background: var(--bg-base, #18181b);
    padding: 1.5rem;
  }

  .card {
    width: 100%;
    max-width: 460px;
    padding: 1.5rem;
    background: var(--bg-surface, #27272a);
    border: 1px solid var(--border, #3f3f46);
    border-radius: 8px;
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.4);
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  h1 {
    margin: 0;
    font-size: 1.25rem;
    color: var(--text-strong, #fafafa);
  }

  .subtitle {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-muted, #a1a1aa);
    line-height: 1.5;
  }
</style>
