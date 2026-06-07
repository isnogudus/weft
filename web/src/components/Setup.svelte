<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import LangSwitch from './LangSwitch.svelte'

  let { onDone } = $props()
  let password = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      await api.post('/setup/bootstrap', { password })
      await onDone()
    } catch (err) {
      error = err.message || t('Einrichtung fehlgeschlagen.')
    } finally {
      busy = false
    }
  }
</script>

<div class="center-page">
  <form class="panel card" onsubmit={submit}>
    <div class="spread">
      <h1>{t('Ersteinrichtung')}</h1>
      <LangSwitch />
    </div>
    <p class="muted">
      {t('Die Grundstruktur (ou=people, ou=groups, Standardgruppe) wird einmalig angelegt. Dazu wird das')}
      <strong>rootpw</strong> {t('aus der')} <code>ldapd.conf</code> {t('benötigt.')}
    </p>
    <label>
      <span>ldapd rootpw</span>
      <input type="password" bind:value={password} autofocus />
    </label>
    {#if error}<p class="error">{error}</p>{/if}
    <button class="primary" type="submit" disabled={busy || !password} style="width:100%">
      {busy ? t('Wird eingerichtet …') : t('Einrichten')}
    </button>
    <p class="muted" style="margin-top:0.8rem">
      {t('Danach melden Sie sich als')} <code>{app.adminUid}</code> {t('mit dem rootpw an.')}
      {t('Der Admin bindet als')} <code>{app.adminDn}</code> {t('– dies muss exakt dem')}
      <code>rootdn</code> {t('in der')} <code>ldapd.conf</code> {t('entsprechen.')}
    </p>
  </form>
</div>
