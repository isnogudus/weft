<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t, i18n } from '../lib/i18n.svelte.js'
  import { generatePassword } from '../lib/password-gen.js'

  let { user, onClose, onSaved } = $props()
  const isNew = !user.uid
  let showPw = $state(false)
  // When the directory's naming attribute is cn, the RDN value must be one of
  // the entry's own cn values (LDAP constraint) -- server-side, cn is forced
  // to equal the identifier regardless of what's submitted (see
  // service.collapseCNToUID), so there is no independently meaningful,
  // separately editable cn/display-name field in this mode.
  const idAttrIsCN = app.meta?.userIdAttr === 'cn'

  let uid = $state(user.uid || '')
  let cn = $state(user.cn || '')
  let sn = $state(user.sn || '')
  let givenName = $state(user.givenName || '')
  let displayName = $state(user.displayName || '')
  let password = $state('')

  let hasPosix = $state(!!user.posix)
  let uidNumber = $state(user.posix?.uidNumber || '')
  let primaryGroup = $state('')
  let homeDirectory = $state(user.posix?.homeDirectory || '')
  let loginShell = $state(user.posix?.loginShell || '')
  let gecos = $state(user.posix?.gecos || '')

  let hasMail = $state(!!user.mail)
  let mail = $state(user.mail?.mail || '')
  let aliases = $state((user.mail?.aliases || []).join('\n'))

  const userAttrs = app.meta?.userAttrs || []
  let extra = $state({ ...(user.extra || {}) })
  const attrLabel = (a) => (i18n.lang === 'de' ? a.labelDe : a.labelEn) || a.attr
  const optionLabel = (o) => (i18n.lang === 'de' ? o.labelDe : o.labelEn) || o.value
  // A value stored before Options was configured (or set by another tool)
  // may not match any entry -- keep it as an extra, clearly-marked option
  // instead of silently switching it to the first configured one on save.
  const isKnownOption = (a, v) => (a.options || []).some((o) => o.value === (v ?? ''))

  let groups = $state([])
  let error = $state('')
  let busy = $state(false)

  const meta = app.meta

  $effect(() => {
    api.get('/groups').then((gs) => {
      groups = gs
      if (!primaryGroup) {
        // preselect group matching current gid, else default
        const match = gs.find((g) => g.gidNumber === user.posix?.gidNumber)
        primaryGroup = match ? match.cn : meta.primaryGroup
      }
    }).catch(() => {})
  })

  function buildPayload() {
    const p = {
      cn: idAttrIsCN ? uid : cn,
      sn,
      givenName: givenName || '',
      displayName: displayName || '',
    }
    if (hasPosix) {
      p.posix = {
        uidNumber: uidNumber ? Number(uidNumber) : 0,
        gidNumber: 0,
        primaryGroup,
        homeDirectory: homeDirectory || '',
        loginShell: loginShell || '',
        gecos: gecos || '',
      }
    }
    if (hasMail) {
      p.mail = {
        mail,
        aliases: aliases.split('\n').map((s) => s.trim()).filter(Boolean),
      }
    }
    if (userAttrs.length) {
      p.extra = {}
      for (const a of userAttrs) p.extra[a.attr] = extra[a.attr] || ''
    }
    return p
  }

  async function submit(e) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      if (isNew) {
        if (password.length > (meta.maxPasswordLength ?? 72)) {
          throw new Error(t('Passwort: höchstens {n} Zeichen.', { n: meta.maxPasswordLength }))
        }
        await api.post('/users', { uid, password, ...buildPayload() })
      } else {
        await api.put('/users/' + user.uid, buildPayload())
      }
      onSaved()
    } catch (err) {
      error = err.message || t('Speichern fehlgeschlagen.')
    } finally {
      busy = false
    }
  }
</script>

<div class="modal-backdrop" onclick={onClose}>
  <form class="modal" onclick={(e) => e.stopPropagation()} onsubmit={submit}>
    <h2>{isNew ? t('Neuer Benutzer') : t('Benutzer {uid} bearbeiten', { uid: user.uid })}</h2>

    {#if isNew}
      <label><span>{idAttrIsCN ? 'cn' : 'uid'}</span><input bind:value={uid} placeholder="z. B. jdoe" /></label>
    {/if}
    <div class="row">
      <label style="flex:1"><span>{t('Vorname (givenName)')}</span><input bind:value={givenName} /></label>
      <label style="flex:1"><span>{t('Nachname (sn) *')}</span><input bind:value={sn} /></label>
    </div>
    {#if !idAttrIsCN}
      <label><span>{t('Anzeigename (cn) *')}</span><input bind:value={cn} /></label>
    {/if}
    <label><span>displayName</span><input bind:value={displayName} /></label>
    {#if isNew}
      <label><span>{t('Passwort *')}</span>
        <span class="row" style="gap:0.4rem">
          <input type={showPw ? 'text' : 'password'} bind:value={password} style="flex:1" />
          <button type="button" onclick={async () => { password = await generatePassword(meta.maxPasswordLength ?? 72); showPw = true }} title={t('Passphrase vorschlagen')}>{t('Vorschlagen')}</button>
        </span>
      </label>
    {/if}

    <fieldset>
      <legend><label style="margin:0"><input type="checkbox" bind:checked={hasPosix} style="width:auto" /> {t('POSIX-Profil (Shell-Account)')}</label></legend>
      {#if hasPosix}
        <div class="row">
          <label style="flex:1"><span>{t('uidNumber (leer = automatisch)')}</span><input type="number" bind:value={uidNumber} placeholder="auto" /></label>
          <label style="flex:1">
            <span>{t('Primärgruppe (gidNumber)')}</span>
            <select bind:value={primaryGroup}>
              {#each groups as g (g.cn)}<option value={g.cn}>{g.cn} ({g.gidNumber})</option>{/each}
            </select>
          </label>
        </div>
        <label><span>homeDirectory</span><input bind:value={homeDirectory} placeholder={meta.homeTemplate} /></label>
        <label><span>loginShell</span><input bind:value={loginShell} placeholder={meta.defaultShell} /></label>
        <label><span>gecos</span><input bind:value={gecos} /></label>
      {/if}
    </fieldset>

    <fieldset>
      <legend><label style="margin:0"><input type="checkbox" bind:checked={hasMail} style="width:auto" /> {t('Mail-Profil')}</label></legend>
      {#if hasMail}
        <label><span>{t('Primäradresse ({attr})', { attr: meta.mailAttr })}</span><input bind:value={mail} placeholder="user@example.org" /></label>
        <label><span>{t('Aliase (eine pro Zeile)')}</span><textarea rows="3" bind:value={aliases}></textarea></label>
      {/if}
    </fieldset>

    {#if userAttrs.length}
      <fieldset>
        <legend>{t('Weitere Attribute')}</legend>
        {#each userAttrs as a (a.attr)}
          <label><span>{attrLabel(a)}{a.required ? ' *' : ''}</span>
            {#if a.options?.length}
              <select bind:value={extra[a.attr]}>
                {#if !isKnownOption(a, extra[a.attr])}
                  <option value={extra[a.attr] ?? ''}>{t('Aktueller Wert (nicht in der Liste): {v}', { v: extra[a.attr] || '—' })}</option>
                {/if}
                {#each a.options as o (o.value)}<option value={o.value}>{optionLabel(o)}</option>{/each}
              </select>
            {:else}
              <input bind:value={extra[a.attr]} />
            {/if}
          </label>
        {/each}
      </fieldset>
    {/if}

    {#if error}<p class="error">{error}</p>{/if}
    <div class="row" style="justify-content:flex-end">
      <button type="button" onclick={onClose}>{t('Abbrechen')}</button>
      <button class="primary" type="submit" disabled={busy}>{isNew ? t('Anlegen') : t('Speichern')}</button>
    </div>
  </form>
</div>
