<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import UserEditor from './UserEditor.svelte'
  import UserDetail from './UserDetail.svelte'
  import ResetPassword from './ResetPassword.svelte'
  import UserGroups from './UserGroups.svelte'
  import ImportUsers from './ImportUsers.svelte'

  let users = $state([])
  let term = $state('')
  let loading = $state(true)
  let error = $state('')

  let page = $state(1)
  let pageSize = $state(25)
  let total = $state(0)
  const totalPages = $derived(Math.max(1, Math.ceil(total / pageSize)))
  // Unfiltered count of every user in the directory, independent of the
  // search term (which narrows `total` above to just the matches).
  let grandTotal = $state(0)

  let editing = $state(null)   // user object or {} for new
  let detail = $state(null)    // user object (read-only view)
  let resetting = $state(null) // uid
  let viewing = $state(null)   // uid (groups modal)
  let importing = $state(false)
  // uid and cn hold the identical value when the directory names entries by
  // cn (see UserEditor.svelte) -- showing both columns would just repeat it.
  const idAttrIsCN = app.meta?.userIdAttr === 'cn'

  async function load() {
    loading = true
    error = ''
    try {
      const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
      if (term) params.set('q', term)
      const resp = await api.get('/users?' + params)
      users = resp.users
      total = resp.total
      // The requested page can end up past the last one (e.g. after a delete
      // shrinks the result set) -- step back rather than show a blank table.
      if (users.length === 0 && page > 1 && total > 0) {
        page = Math.max(1, Math.ceil(total / pageSize))
        return load()
      }
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function search() { page = 1; load() }
  function goToPage(n) { page = Math.min(Math.max(1, n), totalPages); load() }
  function changePageSize(n) { pageSize = n; page = 1; load() }

  // Independent of the (possibly search-filtered) paginated list; refreshed
  // whenever the user count could have changed.
  async function loadGrandTotal() {
    try { grandTotal = (await api.get('/users?pageSize=1')).total }
    catch { /* shown total just stays stale; the main list surfaces errors */ }
  }

  async function remove(u) {
    if (!confirm(t('Benutzer "{uid}" wirklich löschen?', { uid: u.uid }))) return
    try { await api.del('/users/' + u.uid); await load(); await loadGrandTotal() }
    catch (e) { error = e.message }
  }

  function onSaved() { editing = null; load(); loadGrandTotal() }

  $effect(() => { load(); loadGrandTotal() })
</script>

<p class="muted" style="margin:0 0 0.5rem">{t('Insgesamt {n} Benutzer', { n: grandTotal })}</p>
<div class="spread" style="margin-bottom:1rem">
  <input placeholder={t('Suche (uid, Name) …')} bind:value={term} onkeydown={(e) => e.key === 'Enter' && search()} style="max-width:280px" />
  <div class="row">
    <button onclick={() => (importing = true)}>{t('Importieren')}</button>
    <button class="primary" onclick={() => (editing = {})}>{t('Neuer Benutzer')}</button>
  </div>
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
        <tr><th>{idAttrIsCN ? 'cn' : 'uid'}</th>{#if !idAttrIsCN}<th>{t('Name')}</th>{/if}<th>POSIX</th><th>{t('Mail')}</th><th></th></tr>
      </thead>
      <tbody>
        {#each users as u (u.uid)}
          <tr class="clickable" onclick={() => (detail = u)} title={t('Details anzeigen')}>
            <td><strong>{u.uid}</strong></td>
            {#if !idAttrIsCN}<td>{u.cn}</td>{/if}
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
    <div class="spread" style="margin-top:0.8rem">
      <span class="muted">{t('{total} Benutzer, Seite {page} von {totalPages}', { total, page, totalPages })}</span>
      <div class="row">
        <label style="margin:0" class="row">
          <span class="muted" style="margin:0">{t('pro Seite')}</span>
          <select value={pageSize} onchange={(e) => changePageSize(Number(e.target.value))}>
            {#each [25, 50, 100] as n (n)}<option value={n}>{n}</option>{/each}
          </select>
        </label>
        <button onclick={() => goToPage(1)} disabled={page <= 1}>«</button>
        <button onclick={() => goToPage(page - 1)} disabled={page <= 1}>{t('Zurück')}</button>
        <button onclick={() => goToPage(page + 1)} disabled={page >= totalPages}>{t('Weiter')}</button>
        <button onclick={() => goToPage(totalPages)} disabled={page >= totalPages}>»</button>
      </div>
    </div>
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
{#if importing}
  <ImportUsers onClose={() => (importing = false)} onDone={() => { load(); loadGrandTotal() }} />
{/if}
