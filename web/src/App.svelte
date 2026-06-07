<script>
  import { onMount } from 'svelte'
  import { api, setCsrf } from './lib/api.js'
  import { app } from './lib/store.svelte.js'
  import { t } from './lib/i18n.svelte.js'
  import Login from './components/Login.svelte'
  import Setup from './components/Setup.svelte'
  import AdminApp from './components/AdminApp.svelte'
  import SelfService from './components/SelfService.svelte'

  async function boot() {
    app.loading = true
    try {
      const status = await api.get('/setup/status')
      app.reachable = status.reachable
      app.provisioned = status.provisioned
      app.adminUid = status.adminUid || 'admin'
      app.adminDn = status.adminDn || ''
      if (status.reachable && status.provisioned) {
        await loadSession()
      }
    } catch (e) {
      app.reachable = false
    } finally {
      app.loading = false
    }
  }

  async function loadSession() {
    try {
      const me = await api.get('/me')
      setCsrf(me.csrf)
      app.me = me
      app.meta = await api.get('/meta')
    } catch (e) {
      app.me = null // not logged in
    }
  }

  onMount(boot)
</script>

{#if app.loading}
  <div class="center-page"><p class="muted">{t('Lädt …')}</p></div>
{:else if !app.reachable}
  <div class="center-page">
    <div class="panel card">
      <h1>{t('Verbindung fehlgeschlagen')}</h1>
      <p class="muted">{t('Der LDAP-Server ist nicht erreichbar. Bitte Konfiguration und Server prüfen.')}</p>
      <button onclick={boot}>{t('Erneut versuchen')}</button>
    </div>
  </div>
{:else if !app.provisioned}
  <Setup onDone={boot} />
{:else if !app.me}
  <Login onLogin={loadSession} />
{:else if app.me.isAdmin}
  <AdminApp />
{:else}
  <SelfService />
{/if}
