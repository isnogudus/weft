import { describe, expect, it } from 'vitest'
import {
  UID_PATTERN, autoMap, buildContext, byteLen, deriveUid, detectHeaderRow,
  normalizeHeader, resolveUids, rowsToFields, toRowPayload, validateRow,
} from './importModel.js'
import { generatePassword } from './password-gen.js'

const userAttrs = [
  { attr: 'st', labelDe: 'Bundesland', labelEn: 'State' },
  { attr: 'ou', labelDe: 'Dienststelle', labelEn: 'Department' },
  { attr: 'telephoneNumber', labelDe: 'Telefon', labelEn: 'Phone' },
  { attr: 'funkrufname', labelDe: 'Funkrufname', labelEn: 'Call sign' },
]

describe('normalizeHeader', () => {
  it('strips diacritics and separators', () => {
    expect(normalizeHeader(' E-Mail-Adresse ')).toBe('emailadresse')
    expect(normalizeHeader('Landes- / Bundesbehörde')).toBe('landesbundesbehorde')
    expect(normalizeHeader('Einheit / Fachbereich')).toBe('einheitfachbereich')
    expect(normalizeHeader('Straße')).toBe('strasse')
  })
})

describe('autoMap', () => {
  it('maps the German template headers', () => {
    const headers = ['Nr.', 'Bundesland', 'Dienststelle', 'Nachname', 'Vorname',
      'Telefon', 'E-Mail-Adresse', 'Alias', 'Funkrufname', 'Username', 'Passwort']
    expect(autoMap(headers, userAttrs)).toEqual([
      '', 'extra:st', 'extra:ou', 'sn', 'givenName',
      'extra:telephoneNumber', 'mail', 'aliases', 'extra:funkrufname', 'uid', 'password',
    ])
  })
  it('claims each target once and ignores unknown headers', () => {
    expect(autoMap(['uid', 'Benutzername', 'Blume'], [])).toEqual(['uid', '', ''])
  })
  it('drops extra targets that are not configured', () => {
    expect(autoMap(['Bundesland'], [])).toEqual([''])
  })
})

describe('detectHeaderRow', () => {
  it('skips a group band above the real header', () => {
    const rows = [
      ['', 'Verpflichtende Angaben', '', ''],
      ['Nr.', 'Nachname', 'Vorname', 'Username'],
      ['1', 'Müller', 'Anna', 'amueller'],
    ]
    expect(detectHeaderRow(rows, userAttrs)).toBe(1)
  })
})

function ctxFor(fields, existing = []) {
  const meta = { uidMin: 10000, uidMax: 10999, maxPasswordLength: 72, userAttrs }
  return buildContext(meta, existing, [], fields)
}

describe('validateRow', () => {
  const base = { uid: 'anna', sn: 'Müller', givenName: 'Anna', extra: { st: 'NI', ou: 'X', telephoneNumber: '1', funkrufname: 'f' } }

  it('accepts a valid row and derives cn', () => {
    const ctx = ctxFor([base])
    expect(validateRow(base, ctx)).toEqual({ level: 'ok', errors: {} })
  })
  it('rejects a bad uid and duplicates within the file', () => {
    const dup = [{ ...base }, { ...base }]
    const ctx = ctxFor(dup)
    expect(validateRow(dup[0], ctx).errors.uid).toMatch(/doppelt/)
    expect(validateRow({ ...base, uid: 'Über!' }, ctxFor([base])).errors.uid).toMatch(/ungültige/)
  })
  it('flags an existing uid as conflict, not invalid', () => {
    const ctx = ctxFor([base], [{ uid: 'anna' }])
    expect(validateRow(base, ctx).level).toBe('conflict')
  })
  it('checks mail syntax, file duplicates and directory duplicates', () => {
    const withMail = { ...base, mail: 'a@b.co' }
    expect(validateRow({ ...base, mail: 'nope' }, ctxFor([base])).errors.mail).toBeTruthy()
    const ctx = ctxFor([withMail], [{ uid: 'x', mail: { mail: 'A@B.co' } }])
    expect(validateRow(withMail, ctx).errors.mail).toMatch(/gehört bereits x/)
  })

  it('treats an existing uid with a DIFFERENT mail as a decision, not a skip', () => {
    const existing = [{ uid: 'luise.mueller', mail: { mail: 'luise.mueller@x.de' } }]
    // Explicit username collides, but the mail says: probably another person.
    const other = { uid: 'luise.mueller', sn: 'Müller', givenName: 'Luise', mail: 'luise.m2@x.de', extra: {} }
    const res = validateRow(other, ctxFor([other], existing), { index: 0 })
    expect(res.level).toBe('invalid') // must be resolved, NOT silently skipped
    expect(res.errors.uid).toMatch(/andere Person/)
    expect(res.errors.mail).toMatch(/weicht von der Mail im Verzeichnis ab/)

    // Same mail -> same person -> skip as exists.
    const same = { ...other, mail: 'Luise.Mueller@x.de' }
    expect(validateRow(same, ctxFor([same], existing), { index: 0 }).level).toBe('conflict')

    // No mail to compare -> benefit of the doubt, skip as exists.
    const noMail = { ...other, mail: '' }
    expect(validateRow(noMail, ctxFor([noMail], existing), { index: 0 }).level).toBe('conflict')
  })
  it('checks uidNumber range and collisions', () => {
    expect(validateRow({ ...base, uidNumber: '99' }, ctxFor([base])).errors.uidNumber).toBeTruthy()
    const ctx = ctxFor([base], [{ uid: 'x', posix: { uidNumber: 10005 } }])
    expect(validateRow({ ...base, uidNumber: '10005' }, ctx).errors.uidNumber).toMatch(/vergeben/)
  })
  it('enforces required extra attributes and the password byte length', () => {
    const required = [{ attr: 'st', labelDe: 'Bundesland', labelEn: 'State', required: true }]
    const meta = { maxPasswordLength: 72, userAttrs: required }
    const f = { uid: 'anna', sn: 'M', extra: {} }
    const ctx = buildContext(meta, [], [], [f])
    expect(validateRow(f, ctx).errors['extra:st']).toBeTruthy()
    const long = { uid: 'anna', sn: 'M', password: 'ü'.repeat(40), extra: { st: 'NI' } }
    expect(validateRow(long, buildContext(meta, [], [], [long])).errors.password).toBeTruthy()
  })
})

describe('rowsToFields / toRowPayload', () => {
  it('materializes mapped columns including aliases and extra', () => {
    const mapping = ['uid', 'sn', 'aliases', 'extra:st', '']
    const [f] = rowsToFields([['anna', 'Müller', 'a@x.de, b@x.de', 'NI', 'junk']], mapping)
    expect(f).toEqual({ uid: 'anna', sn: 'Müller', aliases: ['a@x.de', 'b@x.de'], extra: { st: 'NI' } })
  })
  it('builds the row payload with posix defaults and configured extras', () => {
    const f = { uid: 'anna', sn: 'Müller', givenName: 'Anna', password: 'pw', extra: { st: 'NI' } }
    const p = toRowPayload(f, 3, { posix: true, primaryGroup: 'users', userAttrs })
    expect(p.row).toBe(3)
    expect(p.cn).toBe('Anna Müller')
    expect(p.posix.primaryGroup).toBe('users')
    expect(p.extra).toEqual({ st: 'NI', ou: '', telephoneNumber: '', funkrufname: '' })
    expect(p.mail).toBeUndefined()
  })
})

describe('deriveUid', () => {
  it('builds vorname.nachname with German transliteration', () => {
    expect(deriveUid('Jörn', 'Peinelt')).toBe('joern.peinelt')
    expect(deriveUid('Benjamin', 'Schöbel')).toBe('benjamin.schoebel')
    expect(deriveUid('Änne', 'Straß')).toBe('aenne.strass')
    expect(deriveUid('Jürgen', 'Örtel')).toBe('juergen.oertel')
  })
  it('keeps hyphens, drops spaces and other characters', () => {
    expect(deriveUid('Karl-Heinz', 'Müller')).toBe('karl-heinz.mueller')
    expect(deriveUid('Anna Maria', "D'Angelo-de Vries")).toBe('annamaria.dangelo-devries')
  })
  it('handles missing parts and enforces the uid pattern', () => {
    expect(deriveUid('', 'Müller')).toBe('mueller')
    expect(deriveUid('Anna', '')).toBe('anna')
    expect(deriveUid('', '')).toBe('')
    expect(deriveUid('123', '456')).toBe('')
    const long = deriveUid('Maximiliane-Alexandra', 'Müller-Lüdenscheidt-Wackersberg')
    expect(long.length).toBeLessThanOrEqual(32)
    expect(long).toMatch(UID_PATTERN)
  })
})

describe('resolveUids', () => {
  const row = (givenName, sn, mail = '', uid = '') => ({ uid, givenName, sn, mail, extra: {} })

  it('suffixes duplicates within the file', () => {
    const fields = [row('Anna', 'Müller'), row('Anna', 'Müller'), row('Anna', 'Müller')]
    const res = resolveUids(fields, [])
    expect(fields.map((f) => f.uid)).toEqual(['anna.mueller', 'anna.mueller2', 'anna.mueller3'])
    expect(fields[0].uidSuffixed).toBeUndefined()
    expect(fields[1].uidSuffixed).toBe(true)
    expect(res).toEqual({ derived: 3, suffixed: 2 })
  })

  it('keeps the collision when the existing user has the same mail (case-insensitive, incl. alias)', () => {
    const existing = [
      { uid: 'anna.mueller', mail: { mail: 'Anna.Mueller@x.de', aliases: ['am@x.de'] } },
    ]
    const same = [row('Anna', 'Müller', 'anna.mueller@X.DE')]
    resolveUids(same, existing)
    expect(same[0].uid).toBe('anna.mueller') // stays colliding -> import skips as "exists"
    expect(same[0].uidSuffixed).toBeUndefined()

    const viaAlias = [row('Anna', 'Müller', 'AM@x.de')]
    resolveUids(viaAlias, existing)
    expect(viaAlias[0].uid).toBe('anna.mueller')
  })

  it('suffixes when the existing user has a different or no mail', () => {
    const existing = [{ uid: 'anna.mueller', mail: { mail: 'other@x.de' } }, { uid: 'anna.mueller2' }]
    const fields = [row('Anna', 'Müller', 'anna2@x.de'), row('Anna', 'Müller')]
    const res = resolveUids(fields, existing)
    // anna.mueller and anna.mueller2 are taken -> 3, then 4.
    expect(fields.map((f) => f.uid)).toEqual(['anna.mueller3', 'anna.mueller4'])
    expect(res.suffixed).toBe(2)
  })

  it('never rewrites explicit usernames and avoids them when suffixing', () => {
    const fields = [row('Anna', 'Müller', '', 'anna.mueller2'), row('Anna', 'Müller')]
    resolveUids(fields, [{ uid: 'anna.mueller' }])
    expect(fields[0].uid).toBe('anna.mueller2')
    expect(fields[1].uid).toBe('anna.mueller3')
  })

  it('does not suffix the same person appearing twice in the file (same mail)', () => {
    const fields = [row('Anna', 'Müller', 'anna@x.de'), row('Anna', 'Müller', 'ANNA@x.de')]
    const res = resolveUids(fields, [])
    expect(fields.map((f) => f.uid)).toEqual(['anna.mueller', 'anna.mueller'])
    expect(fields[1].uidSuffixed).toBeUndefined()
    expect(res.suffixed).toBe(0)

    // Validation then reports the duplicate at the MAIL of the later row only;
    // the derived uid is just a symptom and stays unflagged.
    const ctx = ctxFor(fields)
    const first = validateRow(fields[0], ctx, { index: 0 })
    const second = validateRow(fields[1], ctx, { index: 1 })
    expect(first).toEqual({ level: 'ok', errors: {} })
    expect(second.level).toBe('invalid')
    expect(second.errors.uid).toBeUndefined()
    expect(second.errors.mail).toMatch(/doppelt/)
  })

  it('reports both rows as "exists" when the same directory user is uploaded twice', () => {
    const existing = [{ uid: 'anna.mueller', mail: { mail: 'anna@x.de' } }]
    const fields = [row('Anna', 'Müller', 'anna@x.de'), row('Anna', 'Müller', 'anna@x.de')]
    resolveUids(fields, existing)
    expect(fields.map((f) => f.uid)).toEqual(['anna.mueller', 'anna.mueller'])
    const ctx = ctxFor(fields, existing)
    expect(validateRow(fields[0], ctx, { index: 0 }).level).toBe('conflict')
    expect(validateRow(fields[1], ctx, { index: 1 }).level).toBe('conflict')
  })

  it('re-resolving after a corrected mail turns a suffixed row back into the collision (same person)', () => {
    const existing = [{ uid: 'anna.mueller', mail: { mail: 'anna.mueller@x.de' } }]
    const fields = [row('Anna', 'Müller', 'tippfehler@x.de')]
    resolveUids(fields, existing)
    expect(fields[0].uid).toBe('anna.mueller2')
    expect(fields[0].uidBase).toBe('anna.mueller')

    // The admin fixes the wrong mail; the wizard clears the derived uid and
    // re-runs the decision (this is what ImportUsers.reresolve does).
    fields[0].mail = 'anna.mueller@x.de'
    fields[0].uid = ''
    delete fields[0].uidSuffixed
    const res = resolveUids(fields, existing)
    expect(fields[0].uid).toBe('anna.mueller') // back to "same person" -> exists/skip
    expect(fields[0].uidSuffixed).toBeUndefined()
    expect(res.suffixed).toBe(0)
  })

  it('keeps suffixed uids within 32 chars and the uid pattern', () => {
    const fields = [
      row('Maximiliane-Alexandra', 'Müller-Lüdenscheidt-Wackersberg'),
      row('Maximiliane-Alexandra', 'Müller-Lüdenscheidt-Wackersberg'),
    ]
    resolveUids(fields, [])
    for (const f of fields) {
      expect(f.uid.length).toBeLessThanOrEqual(32)
      expect(f.uid).toMatch(UID_PATTERN)
    }
    expect(fields[1].uid.endsWith('2')).toBe(true)
  })
})

describe('generatePassword', () => {
  it('stays within the byte limit and matches the passphrase shape', async () => {
    for (let i = 0; i < 500; i++) {
      const p = await generatePassword(72)
      expect(byteLen(p)).toBeLessThanOrEqual(72)
      expect(p).toMatch(/^\S+-\S+-\S+-\d+$/)
    }
  })
})
