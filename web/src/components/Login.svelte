<script>
  import { api, setCsrf } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import LangSwitch from './LangSwitch.svelte'

  let { onLogin } = $props()
  let username = $state('')
  let password = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      const me = await api.post('/login', { username, password })
      setCsrf(me.csrf)
      app.me = me
      await onLogin()
    } catch (err) {
      error = err.status === 429 ? t('Zu viele Versuche. Bitte später erneut.') : t('Anmeldung fehlgeschlagen.')
    } finally {
      busy = false
    }
  }
</script>

<div class="center-page">
  <form class="panel card" onsubmit={submit}>
    <div class="spread">
      <h1>weft</h1>
      <LangSwitch />
    </div>
    <p class="muted">{t('LDAP-Benutzerverwaltung')}</p>
    <label>
      <span>{t('Benutzername (uid)')}</span>
      <input bind:value={username} autocomplete="username" autofocus />
    </label>
    <label>
      <span>{t('Passwort')}</span>
      <input type="password" bind:value={password} autocomplete="current-password" />
    </label>
    {#if error}<p class="error">{error}</p>{/if}
    <button class="primary" type="submit" disabled={busy || !username || !password} style="width:100%">
      {busy ? t('Anmelden …') : t('Anmelden')}
    </button>
  </form>
</div>
