<script>
  import { api } from '../lib/api.js'
  import { t } from '../lib/i18n.svelte.js'

  let { uid, onClose } = $props()
  let groups = $state([])
  let user = $state(null)
  let loading = $state(true)

  $effect(() => {
    Promise.all([
      api.get(`/users/${uid}/groups`),
      api.get(`/users/${uid}`),
    ]).then(([gs, u]) => { groups = gs; user = u }).finally(() => (loading = false))
  })

  function isPrimary(g) {
    return user?.posix && g.gidNumber === user.posix.gidNumber
  }
</script>

<div class="modal-backdrop" onclick={onClose}>
  <div class="modal" onclick={(e) => e.stopPropagation()} style="max-width:460px">
    <h2>{t('Gruppen von {uid}', { uid })}</h2>
    {#if loading}
      <p class="muted">{t('Lädt …')}</p>
    {:else if groups.length === 0}
      <p class="muted">{t('Keine Gruppenzugehörigkeiten.')}</p>
    {:else}
      <table>
        <thead><tr><th>{t('Gruppe')}</th><th>gidNumber</th><th>{t('Art')}</th></tr></thead>
        <tbody>
          {#each groups as g (g.cn)}
            <tr>
              <td>{g.cn}</td>
              <td>{g.gidNumber}</td>
              <td>{isPrimary(g) ? t('Primär') : t('Supplementär')}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
    <div class="row" style="justify-content:flex-end; margin-top:1rem">
      <button onclick={onClose}>{t('Schließen')}</button>
    </div>
  </div>
</div>
