<script>
  import { api, setCsrf, endSession } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import LangSwitch from './LangSwitch.svelte'

  // Shown to an admin whose directory has no base structure yet. The session
  // already holds the rootpw, so the bootstrap needs no password of its own.
  let { onDone } = $props()
  let error = $state('')
  let busy = $state(false)

  const meta = $derived(app.meta || {})

  async function submit(e) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      await api.post('/setup/bootstrap')
      await onDone()
    } catch (err) {
      error = err.message || t('Einrichtung fehlgeschlagen.')
    } finally {
      busy = false
    }
  }

  async function logout() {
    try { await api.post('/logout') } catch {}
    endSession()
    setCsrf('')
    app.me = null
  }
</script>

<div class="center-page">
  <form class="panel card" onsubmit={submit}>
    <div class="spread">
      <h1>{t('Ersteinrichtung')}</h1>
      <LangSwitch />
    </div>
    <p class="muted">
      {t('Das Verzeichnis enthält die Grundstruktur noch nicht. Sie wird jetzt einmalig angelegt:')}
    </p>
    <ul class="muted" style="word-break:break-all; padding-left:1.2rem">
      <li><code>ou={meta.peopleOu},{meta.baseDn}</code></li>
      <li><code>ou={meta.groupsOu},{meta.baseDn}</code></li>
      <li><code>cn={meta.primaryGroup},ou={meta.groupsOu},{meta.baseDn}</code></li>
    </ul>
    {#if error}<p class="error">{error}</p>{/if}
    <button class="primary" type="submit" disabled={busy} style="width:100%">
      {busy ? t('Wird eingerichtet …') : t('Einrichten')}
    </button>
    <p class="muted" style="margin-top:0.8rem">
      {t('Angemeldet als')} <code>{app.me.uid}</code> ({t('bindet als')} <code>{app.adminDn}</code>).
      {t('Vorhandene Einträge bleiben unverändert.')}
      <button type="button" onclick={logout}>{t('Abmelden')}</button>
    </p>
  </form>
</div>
