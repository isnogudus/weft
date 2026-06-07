<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'

  let { onClose } = $props()
  let oldPassword = $state('')
  let newPassword = $state('')
  let confirm = $state('')
  let error = $state('')
  let ok = $state(false)
  let busy = $state(false)

  const max = $derived(app.meta?.maxPasswordLength ?? 72)

  async function submit(e) {
    e.preventDefault()
    error = ''
    if (newPassword !== confirm) { error = t('Passwörter stimmen nicht überein.'); return }
    if (newPassword.length > max) { error = t('Höchstens {n} Zeichen.', { n: max }); return }
    busy = true
    try {
      await api.post('/me/password', { oldPassword, newPassword })
      ok = true
      setTimeout(onClose, 900)
    } catch (err) {
      error = err.status === 401 ? t('Aktuelles Passwort ist falsch.') : (err.message || t('Fehlgeschlagen.'))
    } finally {
      busy = false
    }
  }
</script>

<div class="modal-backdrop" onclick={onClose}>
  <form class="modal" onclick={(e) => e.stopPropagation()} onsubmit={submit}>
    <h2>{t('Eigenes Passwort ändern')}</h2>
    <label><span>{t('Aktuelles Passwort')}</span><input type="password" bind:value={oldPassword} /></label>
    <label><span>{t('Neues Passwort')}</span><input type="password" bind:value={newPassword} /></label>
    <label><span>{t('Neues Passwort bestätigen')}</span><input type="password" bind:value={confirm} /></label>
    {#if error}<p class="error">{error}</p>{/if}
    {#if ok}<p class="ok">{t('Passwort geändert.')}</p>{/if}
    <div class="row" style="justify-content:flex-end">
      <button type="button" onclick={onClose}>{t('Abbrechen')}</button>
      <button class="primary" type="submit" disabled={busy || !oldPassword || !newPassword}>{t('Ändern')}</button>
    </div>
  </form>
</div>
