<script>
  import { api } from '../lib/api.js'
  import { t } from '../lib/i18n.svelte.js'
  import UserEditor from './UserEditor.svelte'
  import UserDetail from './UserDetail.svelte'
  import ResetPassword from './ResetPassword.svelte'
  import UserGroups from './UserGroups.svelte'

  let users = $state([])
  let term = $state('')
  let loading = $state(true)
  let error = $state('')

  let editing = $state(null)   // user object or {} for new
  let detail = $state(null)    // user object (read-only view)
  let resetting = $state(null) // uid
  let viewing = $state(null)   // uid (groups modal)

  async function load() {
    loading = true
    error = ''
    try {
      users = await api.get('/users' + (term ? '?q=' + encodeURIComponent(term) : ''))
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  async function remove(u) {
    if (!confirm(t('Benutzer "{uid}" wirklich löschen?', { uid: u.uid }))) return
    try { await api.del('/users/' + u.uid); await load() }
    catch (e) { error = e.message }
  }

  function onSaved() { editing = null; load() }

  $effect(() => { load() })
</script>

<div class="spread" style="margin-bottom:1rem">
  <input placeholder={t('Suche (uid, Name) …')} bind:value={term} onkeydown={(e) => e.key === 'Enter' && load()} style="max-width:280px" />
  <button class="primary" onclick={() => (editing = {})}>{t('Neuer Benutzer')}</button>
</div>

{#if error}<p class="error">{error}</p>{/if}

<div class="panel">
  {#if loading}
    <p class="muted">{t('Lädt …')}</p>
  {:else if users.length === 0}
    <p class="muted">{t('Keine Benutzer.')}</p>
  {:else}
    <table>
      <thead>
        <tr><th>uid</th><th>{t('Name')}</th><th>POSIX</th><th>{t('Mail')}</th><th></th></tr>
      </thead>
      <tbody>
        {#each users as u (u.uid)}
          <tr class="clickable" onclick={() => (detail = u)} title={t('Details anzeigen')}>
            <td><strong>{u.uid}</strong></td>
            <td>{u.cn}</td>
            <td>{u.posix ? `${u.posix.uidNumber}/${u.posix.gidNumber}` : '—'}</td>
            <td>
              {#if u.mail}
                {u.mail.mail || '—'}{#if u.mail.aliases?.length}<div class="muted">{u.mail.aliases.join(', ')}</div>{/if}
              {:else}—{/if}
            </td>
            <td style="text-align:right; white-space:nowrap" onclick={(e) => e.stopPropagation()}>
              <button onclick={() => (viewing = u.uid)}>{t('Gruppen')}</button>
              <button onclick={() => (resetting = u.uid)}>{t('Passwort')}</button>
              <button onclick={() => (editing = u)}>{t('Bearbeiten')}</button>
              <button class="danger" onclick={() => remove(u)}>{t('Löschen')}</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

{#if detail}
  <UserDetail
    user={detail}
    onClose={() => (detail = null)}
    onEdit={(u) => { detail = null; editing = u }} />
{/if}
{#if editing}
  <UserEditor user={editing} onClose={() => (editing = null)} onSaved={onSaved} />
{/if}
{#if resetting}
  <ResetPassword uid={resetting} onClose={() => (resetting = null)} />
{/if}
{#if viewing}
  <UserGroups uid={viewing} onClose={() => (viewing = null)} />
{/if}
