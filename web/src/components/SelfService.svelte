<script>
  import { api, setCsrf, endSession } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import ChangePassword from './ChangePassword.svelte'
  import LangSwitch from './LangSwitch.svelte'

  let profile = $state(null)
  let groups = $state([])
  let showPw = $state(false)
  let error = $state('')

  async function logout() {
    try { await api.post('/logout') } catch {}
    endSession()
    setCsrf('')
    app.me = null
  }

  $effect(() => {
    api.get('/me/profile').then((p) => (profile = p)).catch((e) => (error = e.message))
    api.get('/me/groups').then((g) => (groups = g)).catch(() => {})
  })
</script>

<header class="appbar">
  <strong>weft</strong>
  <div class="row">
    <span class="muted">{app.me.uid}</span>
    <LangSwitch />
    <button class="primary" onclick={() => (showPw = true)}>{t('Passwort ändern')}</button>
    <button onclick={logout}>{t('Abmelden')}</button>
  </div>
</header>

<div class="container" style="max-width:640px">
  <h1>{t('Mein Profil')}</h1>
  {#if error}<p class="error">{error}</p>{/if}
  {#if profile}
    <div class="panel">
      <table>
        <tbody>
          <tr><th>uid</th><td>{profile.uid}</td></tr>
          <tr><th>{t('Name (cn)')}</th><td>{profile.cn || '—'}</td></tr>
          <tr><th>{t('Nachname (sn)')}</th><td>{profile.sn || '—'}</td></tr>
          <tr><th>{t('Anzeigename')}</th><td>{profile.displayName || '—'}</td></tr>
          {#if profile.posix}
            <tr><th>uidNumber</th><td>{profile.posix.uidNumber}</td></tr>
            <tr><th>Home</th><td>{profile.posix.homeDirectory}</td></tr>
            <tr><th>Shell</th><td>{profile.posix.loginShell}</td></tr>
          {/if}
          {#if profile.mail}
            <tr><th>{t('Mail')}</th><td>{profile.mail.mail || '—'}</td></tr>
            {#if profile.mail.aliases?.length}
              <tr><th>{t('Mail-Aliase')}</th><td>{profile.mail.aliases.join(', ')}</td></tr>
            {/if}
          {/if}
        </tbody>
      </table>
    </div>

    <h2 style="margin-top:1.4rem">{t('Meine Gruppen')}</h2>
    <div class="panel">
      {#if groups.length === 0}
        <p class="muted">{t('Keine.')}</p>
      {:else}
        {#each groups as g (g.cn)}<span class="tag" style="margin-right:0.4rem">{g.cn}</span>{/each}
      {/if}
    </div>
  {:else if !error}
    <p class="muted">{t('Lädt …')}</p>
  {/if}
</div>

{#if showPw}
  <ChangePassword onClose={() => (showPw = false)} />
{/if}
