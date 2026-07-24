<script>
  import { api } from '../lib/api.js'
  import { app } from '../lib/store.svelte.js'
  import { t, i18n } from '../lib/i18n.svelte.js'
  import { parseCSV, toCSV } from '../lib/csv.js'
  import {
    CORE_TARGETS, autoMap, buildContext, byteLen, detectHeaderRow,
    resolveUids, rowsToFields, toRowPayload, userHasAddress, validateRow,
  } from '../lib/importModel.js'
  import { generatePassword } from '../lib/password-gen.js'
  import { generateTestUsers } from '../lib/testUserData.js'

  let { onClose, onDone } = $props()

  const meta = app.meta
  const userAttrs = meta?.userAttrs || []
  const CHUNK = 25

  let step = $state('file') // file | map | review | done
  let error = $state('')
  let fileName = $state('')

  // file step: choose between uploading a file and generating synthetic test
  // users (only offered when config.enable_test_user_generator is set).
  let entryMode = $state('file') // file | generate
  let genGivenName = $state('')
  let genSurname = $state('')
  let genStart = $state(0)
  let genCount = $state(20)
  let genMailDomain = $state('')
  let genUniformPassword = $state(false)
  let genPassword = $state('')

  // map step
  let rawRows = $state([])
  let headerRow = $state(0)
  let mapping = $state([])
  let withPosix = $state(false)
  let primaryGroup = $state(meta?.primaryGroup || '')
  let groups = $state([])

  // review/commit step
  let rows = $state([]) // { index, f, password, check, result, attempted }
  let committing = $state(false)
  let progress = $state(0)
  let existingUsers = []
  let usersByUid = new Map()
  // Generated test users intentionally may share one mail address (several
  // logins for the same real developer/tester) -- validateRow relaxes its
  // mail-uniqueness checks for the batch when this is set.
  let allowDuplicateMail = $state(false)

  // finalizeRows progress: an indeterminate "fetching" phase (paging through
  // existing users), then a determinate "generating passwords" count.
  let finalizing = $state(false)
  let finalizePhase = $state('') // fetch | passwords
  let finalizeDone = $state(0)
  let finalizeTotal = $state(0)

  // done step
  let pwUrl = $state('')

  const attrLabel = (a) => (i18n.lang === 'de' ? a.labelDe : a.labelEn) || a.attr
  const targetOptions = [
    ...CORE_TARGETS.map((c) => ({ value: c, label: c })),
    ...userAttrs.map((a) => ({ value: 'extra:' + a.attr, label: attrLabel(a) })),
  ]

  $effect(() => {
    api.get('/groups').then((gs) => (groups = gs)).catch(() => {})
  })

  async function pickFile(e) {
    const file = e.target.files?.[0]
    if (!file) return
    error = ''
    fileName = file.name
    try {
      if (/\.csv$/i.test(file.name)) {
        rawRows = parseCSV(await file.text())
      } else {
        const XLSX = await import('xlsx')
        const wb = XLSX.read(await file.arrayBuffer(), { type: 'array' })
        const ws = wb.Sheets[wb.SheetNames[0]]
        rawRows = XLSX.utils.sheet_to_json(ws, { header: 1, raw: false, defval: '' })
          .map((r) => r.map((c) => String(c ?? '')))
      }
      if (!rawRows.length) throw new Error(t('Die Datei enthält keine Zeilen.'))
      headerRow = detectHeaderRow(rawRows, userAttrs)
      mapping = autoMap(rawRows[headerRow], userAttrs)
      step = 'map'
    } catch (err) {
      error = err.message || t('Datei konnte nicht gelesen werden.')
    }
  }

  function remap() {
    mapping = autoMap(rawRows[headerRow] || [], userAttrs)
  }

  let derivedCount = $state(0)
  let suffixCount = $state(0)

  async function toReview() {
    error = ''
    if (!mapping.includes('uid') && !(mapping.includes('givenName') && mapping.includes('sn'))) {
      error = t('Keine Spalte ist "uid" zugeordnet (oder alternativ Vor- und Nachname).')
      return
    }
    allowDuplicateMail = false
    const dataRows = rawRows.slice(headerRow + 1)
    await finalizeRows(rowsToFields(dataRows, mapping))
  }

  async function generateAndReview() {
    error = ''
    if (!genGivenName.trim() || !genSurname.trim()) {
      error = t('Vor- und Nachname für die Testbenutzer sind erforderlich.')
      return
    }
    if (!Number.isInteger(genCount) || genCount < 1 || genCount > 500) {
      error = t('Anzahl muss zwischen 1 und 500 liegen.')
      return
    }
    let password = ''
    if (genUniformPassword) {
      if (!genPassword) {
        error = t('Bitte ein einheitliches Passwort eingeben oder eines vorschlagen lassen.')
        return
      }
      if (byteLen(genPassword) > (meta?.maxPasswordLength ?? 72)) {
        error = t('Passwort: höchstens {n} Zeichen.', { n: meta?.maxPasswordLength ?? 72 })
        return
      }
      password = genPassword
    }
    allowDuplicateMail = true
    await finalizeRows(generateTestUsers({
      givenName: genGivenName.trim(), sn: genSurname.trim(),
      start: Number(genStart) || 0, count: genCount,
      mailDomain: genMailDomain.trim(), password, userAttrs,
    }))
  }

  // fetchAllUsers pages through GET /users to collect every existing user --
  // the import wizard needs the full set for collision detection, unlike the
  // Users.svelte table view, which intentionally only shows one page.
  async function fetchAllUsers() {
    const pageSize = 200
    let page = 1
    let all = []
    for (;;) {
      const resp = await api.get(`/users?page=${page}&pageSize=${pageSize}`)
      all = all.concat(resp.users)
      if (all.length >= resp.total || resp.users.length === 0) break
      page++
    }
    return all
  }

  // finalizeRows takes fields from either entry mode (parsed file or
  // generated) and runs the shared uid-collision resolution, password
  // generation and validation before showing the review table.
  async function finalizeRows(fields) {
    finalizing = true
    finalizePhase = 'fetch'
    finalizeDone = 0
    finalizeTotal = fields.length
    try {
      existingUsers = await fetchAllUsers()
      usersByUid = new Map(existingUsers.map((u) => [u.uid, u]))
      const derivedFlags = fields.map((f) => !f.uid)
      const resolved = resolveUids(fields, existingUsers)
      derivedCount = resolved.derived
      suffixCount = resolved.suffixed
      finalizePhase = 'passwords'
      rows = await Promise.all(fields.map(async (f, i) => {
        const password = f.password || await generatePassword(meta?.maxPasswordLength ?? 72)
        finalizeDone++
        return {
          index: i,
          f,
          derived: derivedFlags[i],
          password,
          check: null,
          result: null,
          attempted: false,
        }
      }))
      revalidate()
      step = 'review'
    } catch (err) {
      error = err.message || t('Fehlgeschlagen.')
    } finally {
      finalizing = false
    }
  }

  // reresolve re-runs the uid derivation/collision decision after an edit that
  // can change it (mail or name): a corrected mail may turn a suffixed
  // "Namensvetter" back into "same person -> skip", and vice versa. uids the
  // admin typed manually are left alone; committed rows too.
  function reresolve() {
    for (const r of rows) {
      if (r.derived && !r.f.uidManual && r.result !== 'created') {
        r.f.uid = ''
        delete r.f.uidSuffixed
      }
    }
    const res = resolveUids(rows.map((r) => r.f), existingUsers)
    suffixCount = res.suffixed
    revalidate()
  }

  function revalidate() {
    const ctx = buildContext(meta ?? {}, existingUsers, groups, rows.map((r) => ({ ...r.f, password: r.password })), { allowDuplicateMail })
    rows.forEach((r, i) => {
      r.check = validateRow({ ...r.f, password: r.password }, ctx, { index: i })
    })
  }

  const committable = $derived(rows.filter((r) => r.check?.level === 'ok' && r.result !== 'created'))
  const created = $derived(rows.filter((r) => r.result === 'created'))
  const conflicts = $derived(rows.filter((r) => r.check?.level === 'conflict' || r.result === 'exists'))
  // "failed" includes rows that were never sent because validation blocked them.
  const failed = $derived(rows.filter((r) =>
    ['invalid', 'error', 'skipped', 'unknown'].includes(r.result) ||
    (!r.result && r.check?.level === 'invalid')))

  async function commit() {
    committing = true
    error = ''
    progress = 0
    const todo = committable
    const opts = { posix: withPosix, primaryGroup, userAttrs }
    try {
      for (let i = 0; i < todo.length; i += CHUNK) {
        const chunk = todo.slice(i, i + CHUNK)
        for (const r of chunk) r.attempted = true
        let resp
        try {
          resp = await api.post('/users/import', {
            rows: chunk.map((r) => toRowPayload({ ...r.f, password: r.password }, r.index, opts)),
          })
        } catch (err) {
          for (const r of chunk) if (!r.result) r.result = 'unknown'
          throw err
        }
        for (const res of resp.results) {
          const r = rows.find((x) => x.index === res.row)
          if (!r) continue
          // A row we sent (and pre-validated as free) that answers "exists" was
          // created by an interrupted earlier attempt -- its password is ours.
          if (res.status === 'exists' && r.attempted && r.check?.level === 'ok') r.result = 'created'
          else r.result = res.status
          if (res.error) r.check = { level: 'invalid', errors: { server: res.error } }
        }
        progress = Math.min(i + CHUNK, todo.length)
      }
      finish()
    } catch (err) {
      error = (err.message || t('Fehlgeschlagen.')) + ' — ' + t('„Import starten“ setzt beim fehlgeschlagenen Block fort.')
    } finally {
      committing = false
    }
  }

  function finish() {
    if (created.length) {
      const csv = '﻿' + toCSV([['uid', 'password'], ...created.map((r) => [r.f.uid, r.password])])
      if (pwUrl) URL.revokeObjectURL(pwUrl)
      pwUrl = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
    }
    step = 'done'
  }

  function close() {
    if (pwUrl) URL.revokeObjectURL(pwUrl)
    if (created.length) onDone?.()
    onClose()
  }

  // A column is shown either because a file column was mapped to it, or
  // (for generated rows, which have no mapping) some row actually carries a
  // value for it -- otherwise generated extra attributes would be invisible
  // and uneditable in the review table.
  const hasValue = (get) => rows.some((r) => get(r.f))
  const shownTargets = $derived([
    'uid', 'givenName', 'sn',
    ...(mapping.includes('cn') ? ['cn'] : []),
    ...(mapping.includes('displayName') ? ['displayName'] : []),
    ...(mapping.includes('mail') || mapping.includes('aliases') || hasValue((f) => f.mail) ? ['mail'] : []),
    ...(withPosix && mapping.includes('uidNumber') ? ['uidNumber'] : []),
    ...userAttrs
      .filter((a) => mapping.includes('extra:' + a.attr) || hasValue((f) => f.extra?.[a.attr]))
      .map((a) => 'extra:' + a.attr),
    'password',
  ])
  function targetHeading(tgt) {
    if (tgt === 'password') return t('Passwort')
    if (tgt.startsWith('extra:')) {
      const a = userAttrs.find((x) => 'extra:' + x.attr === tgt)
      return a ? attrLabel(a) : tgt.slice(6)
    }
    return tgt
  }
  function statusBadge(r) {
    if (r.result === 'created') return ['ok', t('angelegt')]
    if (r.result === 'exists' || r.check?.level === 'conflict') return ['warn', t('existiert')]
    if (r.result === 'unknown') return ['warn', t('unbekannt (Verbindung unterbrochen)')]
    if (r.result === 'skipped') return ['warn', t('übersprungen')]
    if (r.result === 'error') return ['err', t('Fehler')]
    if (r.check?.level === 'invalid') return ['err', t('fehlerhaft')]
    return ['ok', 'OK']
  }

  // rowErrors returns the visible reasons a row is not accepted, as
  // [fieldLabel, message] pairs (empty for accepted rows).
  function rowErrors(r) {
    if (r.result === 'created' || !r.check?.errors) return []
    return Object.entries(r.check.errors).map(([k, msg]) => [errorLabel(k), t(msg)])
  }
  function errorLabel(key) {
    if (key === 'server') return t('Server')
    if (key.startsWith('extra:')) return targetHeading(key)
    return key
  }
  // fieldError returns the message for one displayed column, so the offending
  // input itself can be highlighted. Conflict rows ("existiert") are skips,
  // not errors -- their fields stay unhighlighted; the status cell explains.
  function fieldError(r, tgt) {
    if (r.check?.level === 'conflict') return ''
    return r.check?.errors?.[tgt] ?? ''
  }

  // dirUser returns the directory entry a row collides with (via its uid, or
  // via the base name for suffixed rows). Its uid/mail are rendered directly
  // BENEATH the row's own fields so both values sit next to each other.
  function dirUser(r) {
    if (r.f.uidSuffixed && r.f.uidBase) return usersByUid.get(r.f.uidBase)
    return usersByUid.get(r.f.uid)
  }
  function collisionMail(r) {
    if (!r.f.uidSuffixed || !r.f.uidBase) return null
    const ex = usersByUid.get(r.f.uidBase)
    if (!ex) return null
    return { uid: ex.uid, mail: ex.mail?.mail || t('ohne Mail') }
  }
  // mailDiff: does the row's mail differ from the directory entry's addresses
  // (aliases included)? Drives the amber highlight of the comparison line.
  function mailDiff(r) {
    const ex = dirUser(r)
    if (!ex) return false
    if (!r.f.mail && !ex.mail?.mail) return false
    return !(r.f.mail && userHasAddress(ex, r.f.mail))
  }
</script>

{#snippet finalizingStatus()}
  {#if finalizing}
    <p class="muted">
      {finalizePhase === 'fetch'
        ? t('Lade bestehende Benutzer …')
        : t('Erzeuge Passwörter … {done}/{total}', { done: finalizeDone, total: finalizeTotal })}
    </p>
  {/if}
{/snippet}

<div class="modal-backdrop" onclick={close}>
  <div class="modal wide" onclick={(e) => e.stopPropagation()}>
    <h2>{t('Benutzer importieren')}</h2>

    {#if step === 'file'}
      {#if meta?.testUserGenerator}
        <div class="row" style="margin-bottom:0.8rem">
          <button class:active={entryMode === 'file'} onclick={() => (entryMode = 'file')}>{t('Datei hochladen')}</button>
          <button class:active={entryMode === 'generate'} onclick={() => (entryMode = 'generate')}>{t('Testbenutzer generieren')}</button>
        </div>
      {/if}
      {#if entryMode === 'generate'}
        <p class="muted">{t('Erzeugt eine Reihe synthetischer Testbenutzer (z. B. anna_m000, anna_m001, …) mit zufälligen Werten für die konfigurierten Zusatzattribute — nur für Tests/Demos.')}</p>
        <div class="row" style="flex-wrap:wrap">
          <label style="flex:1"><span>{t('Vorname (givenName)')}</span><input bind:value={genGivenName} /></label>
          <label style="flex:1"><span>{t('Nachname (sn) *')}</span><input bind:value={genSurname} /></label>
        </div>
        <div class="row" style="flex-wrap:wrap; align-items:flex-end">
          <label style="max-width:8ch"><span>{t('Start')}</span><input type="number" min="0" bind:value={genStart} /></label>
          <label style="max-width:8ch"><span>{t('Anzahl')}</span><input type="number" min="1" max="500" bind:value={genCount} /></label>
          <label style="flex:1"><span>{t('Mail-Domain (optional)')}</span><input bind:value={genMailDomain} placeholder="beispiel.de" /></label>
        </div>
        <label style="margin:0 0 0.6rem"><input type="checkbox" bind:checked={genUniformPassword} style="width:auto" /> {t('Einheitliches Passwort für alle Testbenutzer verwenden')}</label>
        {#if genUniformPassword}
          <label><span>{t('Passwort')}</span>
            <span class="row" style="gap:0.4rem">
              <input type="text" bind:value={genPassword} style="flex:1" />
              <button type="button" onclick={async () => (genPassword = await generatePassword(meta?.maxPasswordLength ?? 72))} title={t('Passphrase vorschlagen')}>{t('Vorschlagen')}</button>
            </span>
          </label>
        {/if}
        {#if error}<p class="error">{error}</p>{/if}
        {@render finalizingStatus()}
        <div class="row" style="justify-content:flex-end">
          <button class="primary" onclick={generateAndReview} disabled={finalizing}>{t('Generieren und prüfen')}</button>
        </div>
      {:else}
        <p class="muted">{t('CSV-, Excel- (.xlsx) oder Numbers-Datei wählen. Die Datei wird im Browser gelesen; erst der Import überträgt Daten.')}</p>
        <input type="file" accept=".csv,.xlsx,.numbers" onchange={pickFile} />
      {/if}
    {:else if step === 'map'}
      <p class="muted">{fileName} — {t('{n} Zeilen', { n: rawRows.length - headerRow - 1 })}</p>
      <div class="row" style="align-items:flex-end; flex-wrap:wrap">
        <label style="max-width:180px"><span>{t('Kopfzeile (Zeile Nr.)')}</span>
          <input type="number" min="1" max={rawRows.length} value={headerRow + 1}
            onchange={(e) => { headerRow = Math.max(0, Number(e.target.value) - 1); remap() }} />
        </label>
        <label style="margin:0"><input type="checkbox" bind:checked={withPosix} style="width:auto" /> {t('POSIX-Profil anlegen')}</label>
        {#if withPosix}
          <label style="max-width:220px"><span>{t('Primärgruppe')}</span>
            <select bind:value={primaryGroup}>
              {#each groups as g (g.cn)}<option value={g.cn}>{g.cn} ({g.gidNumber})</option>{/each}
            </select>
          </label>
        {/if}
      </div>
      <div class="panel import-table" style="overflow-x:auto; max-height:65vh">
        <table>
          <thead>
            <tr>
              {#each rawRows[headerRow] || [] as h, col (col)}
                <th>
                  <div class="muted" style="font-weight:normal">{h || '—'}</div>
                  <select bind:value={mapping[col]}>
                    <option value="">{t('— ignorieren —')}</option>
                    {#each targetOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
                  </select>
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each rawRows.slice(headerRow + 1, headerRow + 4) as r, i (i)}
              <tr>{#each rawRows[headerRow] as _, col (col)}<td>{r[col] ?? ''}</td>{/each}</tr>
            {/each}
          </tbody>
        </table>
      </div>
      {@render finalizingStatus()}
      <div class="row" style="justify-content:flex-end">
        <button onclick={() => (step = 'file')} disabled={finalizing}>{t('Zurück')}</button>
        <button class="primary" onclick={toReview} disabled={finalizing}>{t('Weiter zur Überprüfung')}</button>
      </div>
    {:else if step === 'review'}
      <p class="muted">
        {t('{n} Zeilen; {ok} bereit, {conflict} übersprungen (uid existiert), {invalid} fehlerhaft.', {
          n: rows.length,
          ok: committable.length,
          conflict: conflicts.length,
          invalid: rows.filter((r) => r.check?.level === 'invalid').length,
        })}
        {t('Passwörter wurden generiert, wo die Datei keine enthielt; alle Felder sind editierbar.')}
        {#if derivedCount}
          {t('{n} uids wurden aus Vorname.Nachname abgeleitet.', { n: derivedCount })}
        {/if}
        {#if suffixCount}
          <strong>{t('{n} davon mit Zahlensuffix wegen Namenskollision — bitte prüfen (gelb markiert).', { n: suffixCount })}</strong>
        {/if}
      </p>
      <div class="panel import-table" style="overflow:auto; max-height:65vh">
        <table>
          <thead>
            <tr>
              <th></th>
              {#each shownTargets as tgt (tgt)}<th>{targetHeading(tgt)}</th>{/each}
              <th>{t('Status')}</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as r (r.index)}
              {@const [cls, label] = statusBadge(r)}
              <tr style={r.check?.level === 'conflict' || r.result === 'created' ? 'opacity:0.55' : ''}>
                <td class="muted">{r.index + 1}</td>
                {#each shownTargets as tgt (tgt)}
                  <td>
                    {#if r.result === 'created'}
                      {tgt === 'password' ? '••••' : tgt.startsWith('extra:') ? r.f.extra?.[tgt.slice(6)] ?? '' : r.f[tgt] ?? ''}
                    {:else if tgt === 'password'}
                      <span style="white-space:nowrap; display:inline-flex; gap:0.25rem">
                        <input style="min-width:24ch" class:invalid={fieldError(r, 'password')} title={fieldError(r, 'password')} bind:value={r.password} oninput={revalidate} />
                        <button type="button" title={t('Neues Passwort vorschlagen')}
                          onclick={async () => { r.password = await generatePassword(meta?.maxPasswordLength ?? 72); revalidate() }}>↻</button>
                      </span>
                    {:else if tgt === 'uid'}
                      <input style="min-width:14ch" class:suffixed={r.f.uidSuffixed} class:invalid={fieldError(r, 'uid')}
                        title={fieldError(r, 'uid') || (r.f.uidSuffixed ? t('Automatisch mit Zahlensuffix versehen: Es gibt schon einen Benutzer dieses Namens mit anderer Mail-Adresse.') : '')}
                        bind:value={r.f.uid}
                        oninput={() => {
                          if (r.f.uid) { r.f.uidManual = true; r.f.uidSuffixed = false; revalidate() }
                          else { r.f.uidManual = false; reresolve() } // cleared -> back to automatic
                        }} />
                      {#if dirUser(r)}
                        <div class="dir-line" class:diff={dirUser(r).uid !== r.f.uid} title={t('Im Verzeichnis vorhandener Benutzer')}>
                          {t('Verzeichnis:')} {dirUser(r).uid}
                        </div>
                      {/if}
                    {:else if tgt === 'mail'}
                      <input style="min-width:22ch" class:invalid={fieldError(r, 'mail')} class:suffixed={collisionMail(r)}
                        title={fieldError(r, 'mail') || (collisionMail(r)
                          ? t('Entscheidet über die Namenskollision: {uid} hat die Mail {mail}. Dieselbe Mail eintragen, wenn es dieselbe Person ist — die Zeile wird dann übersprungen.', collisionMail(r))
                          : '')}
                        bind:value={r.f.mail} oninput={reresolve} />
                      {#if dirUser(r)}
                        <div class="dir-line" class:diff={mailDiff(r)} title={t('Im Verzeichnis vorhandener Benutzer')}>
                          {t('Verzeichnis:')} {dirUser(r).mail?.mail || t('ohne Mail')}
                        </div>
                      {/if}
                    {:else if tgt.startsWith('extra:')}
                      <input style="min-width:12ch" class:invalid={fieldError(r, tgt)} title={fieldError(r, tgt)} bind:value={r.f.extra[tgt.slice(6)]} oninput={revalidate} />
                    {:else}
                      <input style="min-width:14ch" class:invalid={fieldError(r, tgt)} title={fieldError(r, tgt)} bind:value={r.f[tgt]} oninput={reresolve} />
                    {/if}
                  </td>
                {/each}
                <td class="status-cell">
                  <span class="tag" class:danger={cls === 'err'}>{label}</span>
                  {#each rowErrors(r) as [field, msg] (field)}
                    <div class="err-line"><strong>{field}:</strong> {msg}</div>
                  {/each}
                  {#if collisionMail(r)}
                    <div class="warn-line">{t('Namenskollision — bitte prüfen')}</div>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if error}<p class="error">{error}</p>{/if}
      {#if committing}
        <p class="muted">{t('Importiere … {done}/{total}', { done: progress, total: committable.length })}</p>
      {/if}
      <div class="row" style="justify-content:flex-end">
        <button onclick={() => (step = 'map')} disabled={committing}>{t('Zurück')}</button>
        <button class="primary" onclick={commit} disabled={committing || committable.length === 0}>
          {t('Import starten ({n} Benutzer)', { n: committable.length })}
        </button>
      </div>
    {:else if step === 'done'}
      <p>
        {t('{created} angelegt, {skipped} übersprungen (bereits vorhanden), {failed} fehlgeschlagen.', {
          created: created.length, skipped: conflicts.length, failed: failed.length,
        })}
      </p>
      {#if created.length}
        <p class="error">{t('Die generierten Passwörter sind NUR JETZT abrufbar und werden nirgends gespeichert.')}</p>
        <p><a href={pwUrl} download="weft-passwoerter.csv"><button class="primary" type="button">{t('Passwörter als CSV herunterladen')}</button></a></p>
      {/if}
      {#if failed.length}
        <p class="muted">{t('Fehlgeschlagene Zeilen können nach Korrektur erneut importiert werden.')}</p>
        <button onclick={() => (step = 'review')}>{t('Zurück zur Überprüfung')}</button>
      {/if}
      <div class="row" style="justify-content:flex-end">
        <button class="primary" onclick={close}>{t('Schließen')}</button>
      </div>
    {/if}
  </div>
</div>

<style>
  /* Table-heavy dialog: tighter cells and inputs than the global defaults, so
     more columns fit before horizontal scrolling kicks in. */
  .import-table :is(th, td) {
    padding: 0.3rem 0.35rem;
    white-space: nowrap;
  }
  .import-table input,
  .import-table select {
    padding: 0.3rem 0.4rem;
  }
  /* Auto-suffixed uid: needs a conscious look before committing. */
  .import-table input.suffixed {
    border-color: #c77d00;
    background: #fff7e6;
  }
  /* Rejected value: the field itself shows where the problem is … */
  .import-table input.invalid {
    border-color: var(--danger);
    background: #fdecea;
  }
  /* … and the status cell spells out every reason. */
  .import-table .status-cell {
    white-space: normal;
    max-width: 26ch;
  }
  .err-line {
    color: var(--danger);
    font-size: 0.78rem;
    line-height: 1.3;
    margin-top: 0.15rem;
  }
  .warn-line {
    color: #935c00;
    font-size: 0.78rem;
    line-height: 1.3;
    margin-top: 0.15rem;
  }
  /* Directory counterpart, shown right beneath the row's own value. */
  .dir-line {
    color: var(--muted);
    font-size: 0.75rem;
    line-height: 1.3;
    margin-top: 0.1rem;
    white-space: nowrap;
  }
  .dir-line.diff {
    color: #935c00;
  }
  .tag.danger {
    color: var(--danger);
    background: #fdecea;
  }
</style>
