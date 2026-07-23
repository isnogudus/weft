// Pure logic for the bulk import wizard: header auto-mapping, row validation
// and payload building. Kept DOM-free so it is unit-testable; the server-side
// validation (internal/service) stays authoritative — mismatches here only
// cost UX, never integrity.

// Mirrors namePattern in internal/service/validate.go.
export const UID_PATTERN = /^[a-z_][a-z0-9_.-]{0,31}$/

// Import targets besides the configured extra attributes (those use the
// target string `extra:<attr>`). '' means "ignore this column".
export const CORE_TARGETS = [
  'uid', 'givenName', 'sn', 'cn', 'displayName', 'mail', 'aliases', 'password',
  'uidNumber', 'gidNumber', 'homeDirectory', 'loginShell', 'gecos',
]

// normalizeHeader lowercases, strips diacritics (ä -> a) and drops
// space/-/_/. so "E-Mail-Adresse" matches "emailadresse".
export function normalizeHeader(s) {
  return String(s ?? '')
    .toLowerCase()
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .replace(/ß/g, 'ss')
    .replace(/[ \-_./]/g, '')
}

// Header aliases (normalized). First match wins; each target is assignable
// once. Includes the columns of the user's Numbers template (Bundesland,
// Dienststelle, …) mapped onto the standard LDAP attributes.
const HEADER_ALIASES = {
  uid: ['uid', 'username', 'benutzername', 'login', 'loginname', 'user', 'kennung', 'konto'],
  sn: ['sn', 'nachname', 'surname', 'lastname', 'familienname', 'name'],
  givenName: ['givenname', 'vorname', 'firstname', 'rufname'],
  cn: ['cn', 'commonname', 'vollstandigername'],
  displayName: ['displayname', 'anzeigename'],
  mail: ['mail', 'email', 'emailadresse', 'mailadresse'],
  aliases: ['aliases', 'aliase', 'alias', 'mailalias', 'mailaliase'],
  password: ['password', 'passwort', 'kennwort', 'passphrase'],
  uidNumber: ['uidnumber'],
  gidNumber: ['gidnumber'],
  homeDirectory: ['homedirectory', 'home'],
  loginShell: ['loginshell', 'shell'],
  gecos: ['gecos'],
  'extra:st': ['st', 'bundesland'],
  'extra:o': ['o', 'landesbundesbehorde', 'behorde', 'organisation', 'organization'],
  'extra:l': ['l', 'dienstort', 'ort'],
  'extra:ou': ['ou', 'dienststelle'],
  'extra:departmentNumber': ['departmentnumber', 'einheitfachbereich', 'einheit', 'fachbereich'],
  'extra:title': ['title', 'dienstfunktion', 'funktion'],
  'extra:telephoneNumber': ['telephonenumber', 'telefon', 'telefonnummer', 'phone'],
  'extra:labeledURI': ['labeleduri', 'url'],
}

// aliasTable builds the lookup for the configured extra attributes: their LDAP
// name and both UI labels count as aliases, on top of the built-ins above.
// Extra targets whose attribute is not configured are dropped.
export function aliasTable(userAttrs = []) {
  const configured = new Set(userAttrs.map((a) => a.attr))
  const table = {}
  for (const [target, names] of Object.entries(HEADER_ALIASES)) {
    if (target.startsWith('extra:') && !configured.has(target.slice(6))) continue
    table[target] = [...names]
  }
  for (const a of userAttrs) {
    const target = 'extra:' + a.attr
    const names = new Set(table[target] ?? [])
    for (const n of [a.attr, a.labelDe, a.labelEn]) {
      const norm = normalizeHeader(n)
      if (norm) names.add(norm)
    }
    table[target] = [...names]
  }
  return table
}

// autoMap assigns a target to every header column ('' = ignore). First alias
// match wins; every target is claimed at most once.
export function autoMap(headers, userAttrs = []) {
  const table = aliasTable(userAttrs)
  const claimed = new Set()
  return headers.map((h) => {
    const norm = normalizeHeader(h)
    if (!norm) return ''
    for (const [target, names] of Object.entries(table)) {
      if (claimed.has(target)) continue
      if (names.includes(norm)) {
        claimed.add(target)
        return target
      }
    }
    return ''
  })
}

// detectHeaderRow returns the index of the first row where at least two cells
// match a known alias — the user's Numbers template has a group band ABOVE the
// real header row, so row 0 is not necessarily it.
export function detectHeaderRow(rows, userAttrs = [], maxScan = 5) {
  const table = aliasTable(userAttrs)
  const all = new Set(Object.values(table).flat())
  for (let i = 0; i < Math.min(rows.length, maxScan); i++) {
    const hits = rows[i].filter((c) => all.has(normalizeHeader(c))).length
    if (hits >= 2) return i
  }
  return 0
}

// rowsToFields materializes data rows into field objects according to the
// column mapping. aliases cells may hold several addresses (comma/;/space).
export function rowsToFields(rows, mapping) {
  return rows.map((cells) => {
    const f = { extra: {} }
    mapping.forEach((target, col) => {
      if (!target) return
      const v = String(cells[col] ?? '').trim()
      if (target.startsWith('extra:')) f.extra[target.slice(6)] = v
      else if (target === 'aliases') f.aliases = v.split(/[,;\s]+/).filter(Boolean)
      else f[target] = v
    })
    return f
  })
}

export const byteLen = (s) => new TextEncoder().encode(s ?? '').length

// asciifyName lowercases one name part and reduces it to the uid charset:
// German transliteration first (ä→ae, ö→oe, ü→ue, ß→ss — matching the common
// mail-address convention), then remaining diacritics stripped, then anything
// outside [a-z0-9-] (spaces, apostrophes, …) dropped.
function asciifyName(s) {
  return String(s ?? '')
    .toLowerCase()
    .replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue').replace(/ß/g, 'ss')
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .replace(/[^a-z0-9-]/g, '')
}

// deriveUid proposes "vorname.nachname" for rows without a username. The
// result always satisfies UID_PATTERN (leading letter, max 32 chars) or is ''
// when the names yield nothing usable.
export function deriveUid(givenName, sn) {
  let uid = [asciifyName(givenName), asciifyName(sn)].filter(Boolean).join('.')
  uid = uid.replace(/^[^a-z_]+/, '').slice(0, 32).replace(/[.-]+$/, '')
  return UID_PATTERN.test(uid) ? uid : ''
}

// resolveUids derives uids for every field without one and resolves name
// collisions. A name is no identity: two rows (or a row and a directory
// entry) sharing "anna.mueller" may be different people. Policy:
//   - derived uid matches an existing user WITH the same mail address (primary
//     or alias) -> assume the same person, keep the colliding uid; the import
//     will report the row as "exists" and skip it.
//   - derived uid matches an EARLIER FILE ROW with the same mail -> the same
//     person twice in the file. No suffix (that would fake a second person);
//     the row keeps the colliding uid and validateRow reports the duplicate
//     at the MAIL, which is the actual evidence.
//   - any other collision (within the file, or an existing user with a
//     different/no mail) -> assume a different person, assign the next free
//     numeric suffix (anna.mueller2) and set f.uidSuffixed so the UI flags the
//     row for review. Mail is the best tie-breaker available, but it can be
//     wrong in the source list -- hence the visible flag instead of silence.
// Explicit usernames from the file are never rewritten.
export function resolveUids(fields, existingUsers) {
  const byUid = new Map(existingUsers.map((u) => [u.uid, u]))
  const taken = new Set(existingUsers.map((u) => u.uid))
  const fileRowByUid = new Map() // uid -> earlier field that claimed it
  for (const f of fields) if (f.uid) taken.add(f.uid)

  let derived = 0
  let suffixed = 0
  for (const f of fields) {
    if (f.uid) continue
    const base = deriveUid(f.givenName, f.sn)
    if (!base) continue
    derived++
    f.uidBase = base // remembered so an edited mail can re-run this decision
    const existing = byUid.get(base)
    if (existing && f.mail && userHasAddress(existing, f.mail)) {
      f.uid = base // same person as the directory entry: row becomes "exists"
      fileRowByUid.set(base, f)
      continue
    }
    const earlier = fileRowByUid.get(base)
    if (earlier && f.mail && earlier.mail && earlier.mail.toLowerCase() === f.mail.toLowerCase()) {
      f.uid = base // same person twice in the file: duplicate row, not a namesake
      continue
    }
    if (taken.has(base)) {
      f.uid = nextFreeUid(base, taken)
      f.uidSuffixed = true
      suffixed++
    } else {
      f.uid = base
    }
    taken.add(f.uid)
    fileRowByUid.set(f.uid, f)
  }
  return { derived, suffixed }
}

export function userHasAddress(user, mail) {
  const m = mail.toLowerCase()
  return [user.mail?.mail, ...(user.mail?.aliases ?? [])]
    .filter(Boolean)
    .some((a) => a.toLowerCase() === m)
}

function nextFreeUid(base, taken) {
  for (let n = 2; ; n++) {
    const suffix = String(n)
    const uid = base.slice(0, 32 - suffix.length).replace(/[.-]+$/, '') + suffix
    if (!taken.has(uid)) return uid
  }
}

// buildContext precomputes the duplicate/conflict lookups for validateRow from
// the current directory listing and the full set of parsed rows.
export function buildContext(meta, existingUsers, groups, fields) {
  const count = (map, key) => map.set(key, (map.get(key) ?? 0) + 1)
  // First-occurrence index per value: duplicates are reported on the LATER
  // rows only; the first row of a duplicate pair is fine by itself.
  const first = (map, key, i) => { if (!map.has(key)) map.set(key, i) }
  const fileUids = new Map()
  const fileMails = new Map()
  const fileUidNumbers = new Map()
  const fileUidFirst = new Map()
  const fileMailFirst = new Map()
  fields.forEach((f, i) => {
    if (f.uid) { count(fileUids, f.uid); first(fileUidFirst, f.uid, i) }
    if (f.mail) { count(fileMails, f.mail.toLowerCase()); first(fileMailFirst, f.mail.toLowerCase(), i) }
    if (f.uidNumber) count(fileUidNumbers, String(f.uidNumber))
  })
  // Mail address -> owning uid (primary addresses and aliases).
  const existingMailOwner = new Map()
  for (const u of existingUsers) {
    for (const a of [u.mail?.mail, ...(u.mail?.aliases ?? [])]) {
      if (a) existingMailOwner.set(a.toLowerCase(), u.uid)
    }
  }
  return {
    fileUidFirst,
    fileMailFirst,
    meta,
    groups,
    existingUids: new Set(existingUsers.map((u) => u.uid)),
    existingByUid: new Map(existingUsers.map((u) => [u.uid, u])),
    existingUidNumbers: new Set(existingUsers.filter((u) => u.posix).map((u) => u.posix.uidNumber)),
    existingMailOwner,
    fileUids,
    fileMails,
    fileUidNumbers,
  }
}

// validateRow mirrors the server-side checks (plus advisory duplicate checks
// the server deliberately leaves to the client). Returns
// { level: 'ok'|'conflict'|'invalid', errors: { field: message } }.
// 'conflict' = uid already in the directory (row will be skipped, not fatal).
// opts.index is the row's position; duplicates within the file are reported on
// the later occurrence only, and preferably at the MAIL: with derived uids a
// duplicated uid is just a symptom, the duplicated address is the evidence.
export function validateRow(f, ctx, opts = {}) {
  const errors = {}
  const meta = ctx.meta ?? {}
  const idx = opts.index
  const dupOfEarlier = (firstMap, key) =>
    firstMap != null && idx != null ? firstMap.has(key) && firstMap.get(key) < idx : false
  const mailKey = f.mail?.toLowerCase()
  // Count-based fallback when no index is known (flags every occurrence).
  const mailDup = mailKey && (dupOfEarlier(ctx.fileMailFirst, mailKey) ||
    (idx == null && ctx.fileMails.get(mailKey) > 1))

  if (!f.uid) errors.uid = 'uid fehlt'
  else if (!UID_PATTERN.test(f.uid)) errors.uid = 'ungültige uid'
  else if (ctx.existingUids.has(f.uid)) {
    // The uid is only the identifier -- the mail is the real criterion for
    // "same person". Skip as "exists" only when the mail matches (or the row
    // has none to compare); a differing mail likely means a DIFFERENT person
    // whose uid collides, so force a decision instead of silently skipping.
    const ex = ctx.existingByUid?.get(f.uid)
    if (f.mail && ex && !userHasAddress(ex, f.mail)) {
      errors.uid = 'existiert mit anderer Mail — andere Person? Dann uid ändern'
      errors.mail = 'weicht von der Mail im Verzeichnis ab'
    } else {
      return { level: 'conflict', errors: { uid: 'uid existiert bereits' } }
    }
  } else if (dupOfEarlier(ctx.fileUidFirst, f.uid) || (idx == null && ctx.fileUids.get(f.uid) > 1)) {
    // Suppress the uid message when the same pair also shares the mail: then
    // the row is a duplicate PERSON and the mail error below tells the story.
    if (!mailDup) errors.uid = 'uid doppelt in der Datei'
  }

  if (!f.sn && !opts.deriveNames) errors.sn = 'Nachname (sn) fehlt'
  const cn = f.cn || [f.givenName, f.sn].filter(Boolean).join(' ')
  if (!cn) errors.cn = 'cn fehlt'
  for (const [k, v] of Object.entries({ cn, sn: f.sn ?? '' })) {
    // eslint-disable-next-line no-control-regex
    if (/[\x00-\x1f]/.test(v)) errors[k] = 'Steuerzeichen im Wert'
  }

  if (f.mail && !errors.mail) {
    const owner = ctx.existingMailOwner?.get(mailKey)
    if (!/.+@.+\..+/.test(f.mail)) errors.mail = 'ungültige Adresse'
    else if (mailDup) errors.mail = 'Adresse doppelt in der Datei — vermutlich doppelte Zeile'
    else if (owner && owner !== f.uid) errors.mail = `Adresse gehört bereits ${owner}`
  }

  if (f.uidNumber) {
    const n = Number(f.uidNumber)
    if (!Number.isInteger(n) || n <= 0) errors.uidNumber = 'keine Zahl'
    else if (meta.uidMin && (n < meta.uidMin || n > meta.uidMax)) {
      errors.uidNumber = `außerhalb ${meta.uidMin}–${meta.uidMax}`
    } else if (ctx.fileUidNumbers.get(String(f.uidNumber)) > 1) errors.uidNumber = 'doppelt in der Datei'
    else if (ctx.existingUidNumbers.has(n)) errors.uidNumber = 'bereits vergeben'
  }

  if (f.password && byteLen(f.password) > (meta.maxPasswordLength ?? 72)) {
    errors.password = `Passwort länger als ${meta.maxPasswordLength ?? 72} Bytes`
  }

  for (const a of meta.userAttrs ?? []) {
    if (a.required && !(f.extra?.[a.attr] ?? '').trim()) {
      errors['extra:' + a.attr] = 'Pflichtfeld'
    }
  }

  return { level: Object.keys(errors).length ? 'invalid' : 'ok', errors }
}

// toRowPayload builds the importRowReq body for one validated row.
export function toRowPayload(f, index, opts) {
  const p = {
    row: index,
    uid: f.uid,
    cn: f.cn || [f.givenName, f.sn].filter(Boolean).join(' '),
    sn: f.sn || '',
    givenName: f.givenName || '',
    displayName: f.displayName || '',
    password: f.password,
  }
  if (opts.posix) {
    p.posix = {
      uidNumber: f.uidNumber ? Number(f.uidNumber) : 0,
      gidNumber: f.gidNumber ? Number(f.gidNumber) : 0,
      primaryGroup: opts.primaryGroup || '',
      homeDirectory: f.homeDirectory || '',
      loginShell: f.loginShell || '',
      gecos: f.gecos || '',
    }
  }
  if (f.mail || f.aliases?.length) {
    p.mail = { mail: f.mail || '', aliases: f.aliases || [] }
  }
  if (opts.userAttrs?.length) {
    p.extra = {}
    for (const a of opts.userAttrs) p.extra[a.attr] = f.extra?.[a.attr] || ''
  }
  return p
}
