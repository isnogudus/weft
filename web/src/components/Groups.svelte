<script>
  import { api } from '../lib/api.js'
  import { t } from '../lib/i18n.svelte.js'
  import GroupMembers from './GroupMembers.svelte'

  let groups = $state([])
  let loading = $state(true)
  let error = $state('')

  let newCn = $state('')
  let newGid = $state('')
  let creating = $state(false)
  let managing = $state(null) // cn

  async function load() {
    loading = true; error = ''
    try { groups = await api.get('/groups') }
    catch (e) { error = e.message }
    finally { loading = false }
  }

  async function create(e) {
    e.preventDefault()
    creating = true; error = ''
    try {
      await api.post('/groups', { cn: newCn, gidNumber: newGid ? Number(newGid) : 0 })
      newCn = ''; newGid = ''
      await load()
    } catch (err) { error = err.message } finally { creating = false }
  }

  async function remove(g) {
    if (!confirm(t('Gruppe "{cn}" wirklich löschen?', { cn: g.cn }))) return
    try { await api.del('/groups/' + g.cn); await load() }
    catch (e) { error = e.message }
  }

  $effect(() => { load() })
</script>

<form class="panel spread" onsubmit={create} style="margin-bottom:1rem; gap:0.6rem">
  <input placeholder={t('Neue Gruppe (cn)')} bind:value={newCn} style="max-width:240px" />
  <input type="number" placeholder="gidNumber (auto)" bind:value={newGid} style="max-width:180px" />
  <button class="primary" type="submit" disabled={creating || !newCn}>{t('Anlegen')}</button>
</form>

{#if error}<p class="error">{error}</p>{/if}

<div class="panel">
  {#if loading}
    <p class="muted">{t('Lädt …')}</p>
  {:else if groups.length === 0}
    <p class="muted">{t('Keine Gruppen.')}</p>
  {:else}
    <table>
      <thead><tr><th>cn</th><th>gidNumber</th><th>{t('Mitglieder')}</th><th></th></tr></thead>
      <tbody>
        {#each groups as g (g.cn)}
          <tr>
            <td><strong>{g.cn}</strong></td>
            <td>{g.gidNumber}</td>
            <td>{g.memberUid.length}</td>
            <td style="text-align:right; white-space:nowrap">
              <button onclick={() => (managing = g.cn)}>{t('Mitglieder')}</button>
              <button class="danger" onclick={() => remove(g)}>{t('Löschen')}</button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

{#if managing}
  <GroupMembers cn={managing} onClose={() => { managing = null; load() }} />
{/if}
