// Synthetic test-user generation for the bulk import wizard (gated by
// config.TestUserGenerator / meta.testUserGenerator). Produces the same row
// shape rowsToFields() does, so it feeds straight into the existing
// review/collision/commit pipeline in importModel.js.
//
// Ported from the admin's own ldap.py block-generator script: same naming
// scheme (givenname_s000, zero-padded) and the same flavor of made-up
// German police-themed test data (no real people, places tied to real
// addresses, or real organizations).

// https://de.wikipedia.org/wiki/Liste_fiktiver_deutscher_Ortsnamen -- fictional
// German place names, safe to use as filler "Dienstort" values.
const LOCALITIES = [
  'Alpenfestung', 'Apfelsdorf', 'Barbarswila', 'Bärstadt', 'Bichelheim', 'Büttenwarder',
  'Deekelsen', 'Dusterstadt', 'Entepfuhl', 'Eulenberg', 'Eulenstein', 'Falkenheim',
  'Fingen', 'Finsdorf', 'Fortzenheim', 'Germelshausen', 'Grievow', 'Güllen',
  'Hintertupfingen', 'Hindafing', 'Hengasch', 'Hinterschlag', 'Hunsum', 'Isenstadt',
  'Kälberfurtz', 'Kalmüsel', 'Kana', 'Kanzheim', 'Klein-Beken', 'Kleinstadt',
  'Küblach', 'Kugelstadt', 'Lansing', 'Lastrum', 'Mägdeflecken', 'Meuchelbeck',
  'Mitkau', 'Mügli am See', 'Müglisee', 'Neu-Schaffrath', 'Neustadt',
  'Niederkaltenkirchen', 'Philippsburg', 'Plattengülle', 'Reichenberg', 'Reinöd',
  'Rossitz', 'Ruch', 'Schabbach', 'Schriedingen', 'Schwanitz', 'Seebühl am Bühlsee',
  'Seldwyla', 'Sieghartsweiler', 'Stenkelfeld', 'Tassing', 'Uhlenbusch', 'Utzbach',
  'Unterleuten', 'Volpe', 'Wahlheim', 'Waldau', 'Waldhagen', 'Waldsee', 'Warwand',
  'Weinberg', 'Weissnichtwo', 'Weißfischhausen', 'Wulfburg',
]

const STATES = [
  'Baden-Württemberg', 'Bayern', 'Berlin', 'Brandenburg', 'Bremen', 'Hamburg', 'Hessen',
  'Mecklenburg-Vorpommern', 'Niedersachsen', 'Nordrhein-Westfalen', 'Rheinland-Pfalz',
  'Saarland', 'Sachsen', 'Sachsen-Anhalt', 'Schleswig-Holstein', 'Thüringen',
]

// A handful of real German area codes, just for a plausible-looking fake
// phone number -- not tied to any real subscriber.
const PHONE_AREAS = ['30', '69', '40', '221', '89']

const POLICE_UNITS = ['Polizeiinspektion', 'Polizeistation', 'Polizeikommissariat', 'Polizeidirektion']
const EMPLOYEE_TYPES = ['Schutzpolizei', 'Bereitschaftspolizei', 'Kriminalpolizei']

const TRANSLIT = { ä: 'ae', ö: 'oe', ü: 'ue', ß: 'ss' }

// asciify lowercases and transliterates German umlauts/ß so the result is a
// safe uid component (matches UID_PATTERN's charset once combined and padded).
function asciify(s) {
  return String(s ?? '')
    .toLowerCase()
    .replace(/[äöüß]/g, (c) => TRANSLIT[c])
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .replace(/[^a-z0-9]/g, '')
}

const pick = (arr) => arr[Math.floor(Math.random() * arr.length)]
const digits = (n) => String(Math.floor(Math.random() * 10 ** n)).padStart(n, '0')

// generateTestUsers builds `count` synthetic rows named
// "<givenname>_<n><start+i padded to 3 digits>" (e.g. "anna_m000",
// "anna_m001", ...), in the shape rowsToFields() produces. Only the extra
// attributes actually configured (per userAttrs, from /api/meta) are filled;
// everything else is left for the admin to edit in the review table.
// mailDomain is optional -- blank skips mail entirely.
export function generateTestUsers({ givenName, sn, start = 0, count = 20, mailDomain = '', userAttrs = [] }) {
  const givenAscii = asciify(givenName)
  const snAscii = asciify(sn)
  const configured = new Set(userAttrs.map((a) => a.attr))
  const has = (attr) => configured.has(attr)

  const rows = []
  for (let i = start; i < start + count; i++) {
    const uid = `${givenAscii}_${snAscii[0] || 'x'}${String(i).padStart(3, '0')}`
    const extra = {}
    if (has('st')) extra.st = pick(STATES)
    if (has('l')) extra.l = pick(LOCALITIES)
    if (has('o')) extra.o = `${pick(POLICE_UNITS)} ${pick(LOCALITIES)} ${1 + Math.floor(Math.random() * 9)}`
    if (has('ou')) extra.ou = `${pick(POLICE_UNITS)} ${pick(LOCALITIES)}`
    if (has('departmentNumber')) extra.departmentNumber = `Dezernat ${1 + Math.floor(Math.random() * 9)}`
    if (has('title')) extra.title = pick(EMPLOYEE_TYPES)
    if (has('telephoneNumber')) extra.telephoneNumber = `+49 ${pick(PHONE_AREAS)} ${digits(4)}`

    rows.push({
      uid,
      givenName,
      sn,
      cn: '',
      displayName: '',
      mail: mailDomain ? `${givenAscii}.${snAscii}@${mailDomain}` : '',
      aliases: [],
      extra,
      password: '',
    })
  }
  return rows
}
