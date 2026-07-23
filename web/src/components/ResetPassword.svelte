<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t } from '../lib/i18n.svelte.js'
  import { generatePassword } from '../lib/password-gen.js'

  let { uid, onClose } = $props()
  let newPassword = $state('')
  let confirm = $state('')
  let error = $state('')
  let ok = $state(false)
  let busy = $state(false)
  let show = $state(false)
  const max = $derived(app.meta?.maxPasswordLength ?? 72)

  async function suggest() {
    newPassword = confirm = await generatePassword(max)
    show = true
  }

  async function submit(e) {
    e.preventDefault()
    error = ''
    if (newPassword !== confirm) { error = t('Passwörter stimmen nicht überein.'); return }
    if (newPassword.length > max) { error = t('Höchstens {n} Zeichen.', { n: max }); return }
    busy = true
    try {
      await api.post(`/users/${uid}/password`, { newPassword })
      ok = true
      setTimeout(onClose, 900)
    } catch (err) {
      error = err.message || t('Fehlgeschlagen.')
    } finally {
      busy = false
    }
  }
</script>

<div class="modal-backdrop" onclick={onClose}>
  <form class="modal" onclick={(e) => e.stopPropagation()} onsubmit={submit} style="max-width:420px">
    <h2>{t('Passwort zurücksetzen: {uid}', { uid })}</h2>
    <label><span>{t('Neues Passwort')}</span>
      <span class="row" style="gap:0.4rem">
        <input type={show ? 'text' : 'password'} bind:value={newPassword} autofocus style="flex:1" />
        <button type="button" onclick={suggest} title={t('Passphrase vorschlagen')}>{t('Vorschlagen')}</button>
      </span>
    </label>
    <label><span>{t('Bestätigen')}</span><input type={show ? 'text' : 'password'} bind:value={confirm} /></label>
    {#if error}<p class="error">{error}</p>{/if}
    {#if ok}<p class="ok">{t('Passwort gesetzt.')}</p>{/if}
    <div class="row" style="justify-content:flex-end">
      <button type="button" onclick={onClose}>{t('Abbrechen')}</button>
      <button class="primary" type="submit" disabled={busy || !newPassword}>{t('Setzen')}</button>
    </div>
  </form>
</div>
