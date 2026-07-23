<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t, i18n } from '../lib/i18n.svelte.js'

  let { user, onClose, onEdit } = $props()
  let groups = $state([])
  const userAttrs = app.meta?.userAttrs || []
  const attrLabel = (a) => (i18n.lang === 'de' ? a.labelDe : a.labelEn) || a.attr

  $effect(() => {
    api.get(`/users/${user.uid}/groups`).then((g) => (groups = g)).catch(() => {})
  })

  function isPrimary(g) {
    return user.posix && g.gidNumber === user.posix.gidNumber
  }
</script>

<div class="modal-backdrop" onclick={onClose}>
  <div class="modal" onclick={(e) => e.stopPropagation()}>
    <h2>{t('Benutzer: {uid}', { uid: user.uid })}</h2>

    <div class="panel">
      <table>
        <tbody>
          <tr><th>uid</th><td>{user.uid}</td></tr>
          <tr><th>{t('Vorname')}</th><td>{user.givenName || '—'}</td></tr>
          <tr><th>{t('Nachname (sn)')}</th><td>{user.sn || '—'}</td></tr>
          <tr><th>{t('Name (cn)')}</th><td>{user.cn || '—'}</td></tr>
          <tr><th>{t('Anzeigename')}</th><td>{user.displayName || '—'}</td></tr>
          {#if user.posix}
            <tr><th>uidNumber</th><td>{user.posix.uidNumber}</td></tr>
            <tr><th>gidNumber</th><td>{user.posix.gidNumber}</td></tr>
            <tr><th>homeDirectory</th><td>{user.posix.homeDirectory}</td></tr>
            <tr><th>loginShell</th><td>{user.posix.loginShell}</td></tr>
            {#if user.posix.gecos}<tr><th>gecos</th><td>{user.posix.gecos}</td></tr>{/if}
          {/if}
          {#if user.mail}
            <tr><th>{t('Mail')}</th><td>{user.mail.mail || '—'}</td></tr>
            {#if user.mail.aliases?.length}
              <tr><th>{t('Mail-Aliase')}</th><td>{user.mail.aliases.join(', ')}</td></tr>
            {/if}
          {/if}
          {#each userAttrs as a (a.attr)}
            {#if user.extra?.[a.attr]}
              <tr><th>{attrLabel(a)}</th><td>{user.extra[a.attr]}</td></tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>

    <h2 style="margin-top:1.2rem; font-size:1rem">{t('Gruppen')}</h2>
    <div class="panel">
      {#if groups.length === 0}
        <p class="muted">{t('Keine.')}</p>
      {:else}
        {#each groups as g (g.cn)}
          <span class="tag" style="margin-right:0.4rem">{g.cn}{#if isPrimary(g)} ({t('Primär')}){/if}</span>
        {/each}
      {/if}
    </div>

    <div class="row" style="justify-content:flex-end; margin-top:1rem">
      <button onclick={() => onEdit(user)}>{t('Bearbeiten')}</button>
      <button class="primary" onclick={onClose}>{t('Schließen')}</button>
    </div>
  </div>
</div>
