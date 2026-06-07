<script>
  import { api } from '../lib/api.js'
  import { t } from '../lib/i18n.svelte.js'

  let { cn, onClose } = $props()
  let group = $state(null)
  let newUid = $state('')
  let error = $state('')
  let busy = $state(false)

  async function load() {
    try { group = await api.get('/groups/' + cn) }
    catch (e) { error = e.message }
  }

  async function add(e) {
    e.preventDefault()
    busy = true; error = ''
    try { await api.post(`/groups/${cn}/members`, { uid: newUid }); newUid = ''; await load() }
    catch (err) { error = err.message } finally { busy = false }
  }

  async function remove(uid) {
    try { await api.del(`/groups/${cn}/members/${uid}`); await load() }
    catch (e) { error = e.message }
  }

  $effect(() => { load() })
</script>

<div class="modal-backdrop" onclick={onClose}>
  <div class="modal" onclick={(e) => e.stopPropagation()} style="max-width:440px">
    <h2>{t('Mitglieder: {cn}', { cn })}</h2>
    <p class="muted">{t('Supplementäre Mitglieder (memberUid). Die Primärgruppe wird über die gidNumber am Benutzer gesetzt und erscheint hier nicht.')}</p>

    <form class="row" onsubmit={add} style="margin:0.6rem 0">
      <input placeholder={t('uid hinzufügen')} bind:value={newUid} />
      <button class="primary" type="submit" disabled={busy || !newUid}>{t('Hinzufügen')}</button>
    </form>

    {#if error}<p class="error">{error}</p>{/if}

    {#if group}
      {#if group.memberUid.length === 0}
        <p class="muted">{t('Keine Mitglieder.')}</p>
      {:else}
        <table>
          <tbody>
            {#each group.memberUid as uid (uid)}
              <tr>
                <td>{uid}</td>
                <td style="text-align:right"><button class="danger" onclick={() => remove(uid)}>{t('Entfernen')}</button></td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    {/if}

    <div class="row" style="justify-content:flex-end; margin-top:1rem">
      <button onclick={onClose}>{t('Schließen')}</button>
    </div>
  </div>
</div>
