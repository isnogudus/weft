<script>
  import { api, setCsrf } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import Users from './Users.svelte'
  import Groups from './Groups.svelte'
  import ChangePassword from './ChangePassword.svelte'
  import LangSwitch from './LangSwitch.svelte'

  let tab = $state('users')
  let showPw = $state(false)

  async function logout() {
    try { await api.post('/logout') } catch {}
    setCsrf('')
    app.me = null
  }
</script>

<header class="appbar">
  <strong>weft</strong>
  <div class="row">
    <span class="muted">{app.me.uid} <span class="tag">Admin</span></span>
    <LangSwitch />
    <button onclick={() => (showPw = true)}>{t('Mein Passwort')}</button>
    <button onclick={logout}>{t('Abmelden')}</button>
  </div>
</header>

<div class="container">
  <div class="tabs">
    <button class:active={tab === 'users'} onclick={() => (tab = 'users')}>{t('Benutzer')}</button>
    <button class:active={tab === 'groups'} onclick={() => (tab = 'groups')}>{t('Gruppen')}</button>
  </div>

  {#if tab === 'users'}
    <Users />
  {:else}
    <Groups />
  {/if}
</div>

{#if showPw}
  <ChangePassword onClose={() => (showPw = false)} />
{/if}
