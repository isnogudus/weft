// Lightweight i18n. The German source strings are the keys; only English needs
// a dictionary. t() returns the German key verbatim for `de`, the mapped
// English string for `en` (falling back to the key if unmapped). Placeholders
// look like {name} and are filled from the params object.
//
// The locale is a $state object named `i18n` (NOT `state`, to avoid shadowing
// the $state rune) and is persisted to localStorage.

const STORAGE_KEY = 'weft.lang'

function detect() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'de' || saved === 'en') return saved
  } catch {}
  const nav = (typeof navigator !== 'undefined' && navigator.language) || 'de'
  return nav.toLowerCase().startsWith('en') ? 'en' : 'de'
}

export const i18n = $state({ lang: detect() })

export function setLang(lang) {
  i18n.lang = lang
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {}
}

const en = {
  // App / generic
  'Lädt …': 'Loading …',
  'Verbindung fehlgeschlagen': 'Connection failed',
  'Der LDAP-Server ist nicht erreichbar. Bitte Konfiguration und Server prüfen.':
    'The LDAP server is unreachable. Please check the configuration and the server.',
  'Erneut versuchen': 'Retry',
  'Abbrechen': 'Cancel',
  'Schließen': 'Close',
  'Löschen': 'Delete',
  'Entfernen': 'Remove',
  'Hinzufügen': 'Add',
  'Anlegen': 'Create',
  'Speichern': 'Save',
  'Fehlgeschlagen.': 'Failed.',

  // Login
  'LDAP-Benutzerverwaltung': 'LDAP user management',
  'Benutzername (uid)': 'Username (uid)',
  'Passwort': 'Password',
  'Zu viele Versuche. Bitte später erneut.': 'Too many attempts. Please try again later.',
  'Anmeldung fehlgeschlagen.': 'Login failed.',
  'Anmelden …': 'Signing in …',
  'Anmelden': 'Sign in',
  'Abmelden': 'Sign out',

  // Setup
  'Ersteinrichtung': 'Initial setup',
  'Die Grundstruktur (ou=people, ou=groups, Standardgruppe) wird einmalig angelegt. Dazu wird das':
    'The base structure (ou=people, ou=groups, default group) is created once. This requires the',
  'aus der': 'from',
  'benötigt.': '.',
  'Wird eingerichtet …': 'Setting up …',
  'Einrichten': 'Set up',
  'Einrichtung fehlgeschlagen.': 'Setup failed.',
  'Danach melden Sie sich als': 'Afterwards sign in as',
  'mit dem rootpw an.': 'with the rootpw.',
  'Der Admin bindet als': 'The admin binds as',
  '– dies muss exakt dem': '— this must exactly match the',
  'in der': 'in',
  'entsprechen.': '.',

  // AdminApp
  'Mein Passwort': 'My password',
  'Benutzer': 'Users',
  'Gruppen': 'Groups',

  // Change / reset password
  'Eigenes Passwort ändern': 'Change my password',
  'Aktuelles Passwort': 'Current password',
  'Neues Passwort': 'New password',
  'Neues Passwort bestätigen': 'Confirm new password',
  'Bestätigen': 'Confirm',
  'Passwörter stimmen nicht überein.': 'Passwords do not match.',
  'Höchstens {n} Zeichen.': 'At most {n} characters.',
  'Aktuelles Passwort ist falsch.': 'Current password is incorrect.',
  'Passwort geändert.': 'Password changed.',
  'Passwort gesetzt.': 'Password set.',
  'Ändern': 'Change',
  'Passwort ändern': 'Change password',
  'Passwort zurücksetzen: {uid}': 'Reset password: {uid}',
  'Setzen': 'Set',

  // Users list
  'Suche (uid, Name) …': 'Search (uid, name) …',
  'Neuer Benutzer': 'New user',
  'Bearbeiten': 'Edit',
  'Details anzeigen': 'Show details',
  'Benutzer: {uid}': 'User: {uid}',
  'Vorname': 'First name',
  'Keine Benutzer.': 'No users.',
  'Name': 'Name',
  'Mail': 'Mail',
  'Benutzer "{uid}" wirklich löschen?': 'Really delete user "{uid}"?',

  // User editor
  'Benutzer {uid} bearbeiten': 'Edit user {uid}',
  'Vorname (givenName)': 'First name (givenName)',
  'Nachname (sn) *': 'Surname (sn) *',
  'Anzeigename (cn) *': 'Display name (cn) *',
  'Passwort *': 'Password *',
  'POSIX-Profil (Shell-Account)': 'POSIX profile (shell account)',
  'uidNumber (leer = automatisch)': 'uidNumber (empty = automatic)',
  'Primärgruppe (gidNumber)': 'Primary group (gidNumber)',
  'Mail-Profil': 'Mail profile',
  'Weitere Attribute': 'Additional attributes',
  'Primäradresse ({attr})': 'Primary address ({attr})',
  'Aliase (eine pro Zeile)': 'Aliases (one per line)',
  'Passwort: höchstens {n} Zeichen.': 'Password: at most {n} characters.',
  'Speichern fehlgeschlagen.': 'Saving failed.',

  // User groups modal
  'Gruppen von {uid}': 'Groups of {uid}',
  'Keine Gruppenzugehörigkeiten.': 'No group memberships.',
  'Gruppe': 'Group',
  'Art': 'Type',
  'Primär': 'Primary',
  'Supplementär': 'Supplementary',

  // Groups
  'Neue Gruppe (cn)': 'New group (cn)',
  'Keine Gruppen.': 'No groups.',
  'Mitglieder': 'Members',
  'Gruppe "{cn}" wirklich löschen?': 'Really delete group "{cn}"?',

  // Group members modal
  'Mitglieder: {cn}': 'Members: {cn}',
  'Supplementäre Mitglieder (memberUid). Die Primärgruppe wird über die gidNumber am Benutzer gesetzt und erscheint hier nicht.':
    'Supplementary members (memberUid). The primary group is set via the user\'s gidNumber and does not appear here.',
  'uid hinzufügen': 'add uid',
  'Keine Mitglieder.': 'No members.',

  // Self-service
  'Mein Profil': 'My profile',
  'Name (cn)': 'Name (cn)',
  'Nachname (sn)': 'Surname (sn)',
  'Anzeigename': 'Display name',
  'Mail-Aliase': 'Mail aliases',
  'Meine Gruppen': 'My groups',
  'Keine.': 'None.',

  // Passwords / generator
  'Vorschlagen': 'Suggest',
  'Passphrase vorschlagen': 'Suggest a passphrase',
  'Neues Passwort vorschlagen': 'Suggest a new password',

  // Bulk import
  'Importieren': 'Import',
  'Benutzer importieren': 'Import users',
  'CSV-, Excel- (.xlsx) oder Numbers-Datei wählen. Die Datei wird im Browser gelesen; erst der Import überträgt Daten.':
    'Choose a CSV, Excel (.xlsx) or Numbers file. It is parsed in your browser; nothing is transferred until you start the import.',
  'Die Datei enthält keine Zeilen.': 'The file contains no rows.',
  'Datei konnte nicht gelesen werden.': 'Could not read the file.',
  '{n} Zeilen': '{n} rows',
  'Kopfzeile (Zeile Nr.)': 'Header row (line no.)',
  'POSIX-Profil anlegen': 'Create POSIX profile',
  'Primärgruppe': 'Primary group',
  '— ignorieren —': '— ignore —',
  'Zurück': 'Back',
  'Weiter zur Überprüfung': 'Continue to review',
  'Keine Spalte ist "uid" zugeordnet (oder alternativ Vor- und Nachname).':
    'No column is mapped to "uid" (or, alternatively, first and last name).',
  '{n} uids wurden aus Vorname.Nachname abgeleitet.':
    '{n} uids were derived from firstname.lastname.',
  '{n} davon mit Zahlensuffix wegen Namenskollision — bitte prüfen (gelb markiert).':
    '{n} of them got a numeric suffix due to a name collision — please review (highlighted in yellow).',
  'Automatisch mit Zahlensuffix versehen: Es gibt schon einen Benutzer dieses Namens mit anderer Mail-Adresse.':
    'Automatically suffixed: a user of this name already exists with a different mail address.',
  'Entscheidet über die Namenskollision: {uid} hat die Mail {mail}. Dieselbe Mail eintragen, wenn es dieselbe Person ist — die Zeile wird dann übersprungen.':
    'Decides the name collision: {uid} has the mail {mail}. Enter the same mail if this is the same person — the row will then be skipped.',
  'Namenskollision — bitte prüfen': 'Name collision — please review',
  'Verzeichnis:': 'Directory:',
  'Im Verzeichnis vorhandener Benutzer': 'User already present in the directory',
  'existiert mit anderer Mail — andere Person? Dann uid ändern':
    'exists with a different mail — another person? Then change the uid',
  'weicht von der Mail im Verzeichnis ab': 'differs from the mail in the directory',
  'ohne Mail': 'no mail',
  '{n} Zeilen; {ok} bereit, {conflict} übersprungen (uid existiert), {invalid} fehlerhaft.':
    '{n} rows; {ok} ready, {conflict} skipped (uid exists), {invalid} invalid.',
  'Passwörter wurden generiert, wo die Datei keine enthielt; alle Felder sind editierbar.':
    'Passwords were generated where the file had none; every field is editable.',
  'Status': 'Status',
  'Lade bestehende Benutzer …': 'Loading existing users …',
  'Erzeuge Passwörter … {done}/{total}': 'Generating passwords … {done}/{total}',
  'Importiere … {done}/{total}': 'Importing … {done}/{total}',
  'Import starten ({n} Benutzer)': 'Start import ({n} users)',
  '„Import starten“ setzt beim fehlgeschlagenen Block fort.': '“Start import” resumes at the failed chunk.',
  '{created} angelegt, {skipped} übersprungen (bereits vorhanden), {failed} fehlgeschlagen.':
    '{created} created, {skipped} skipped (already present), {failed} failed.',
  'Die generierten Passwörter sind NUR JETZT abrufbar und werden nirgends gespeichert.':
    'The generated passwords are available ONLY NOW and are not stored anywhere.',
  'Passwörter als CSV herunterladen': 'Download passwords as CSV',
  'Fehlgeschlagene Zeilen können nach Korrektur erneut importiert werden.':
    'Failed rows can be corrected and imported again.',
  'Zurück zur Überprüfung': 'Back to review',
  'angelegt': 'created',
  'existiert': 'exists',
  'unbekannt (Verbindung unterbrochen)': 'unknown (connection lost)',
  'übersprungen': 'skipped',
  'Fehler': 'error',
  'fehlerhaft': 'invalid',
  'Server': 'server',
  // Row validation reasons (from importModel.js validateRow)
  'uid fehlt': 'uid missing',
  'ungültige uid': 'invalid uid',
  'uid doppelt in der Datei': 'duplicate uid in the file',
  'uid existiert bereits': 'uid already exists',
  'Nachname (sn) fehlt': 'surname (sn) missing',
  'cn fehlt': 'cn missing',
  'Steuerzeichen im Wert': 'control characters in value',
  'ungültige Adresse': 'invalid address',
  'Adresse doppelt in der Datei': 'duplicate address in the file',
  'Adresse doppelt in der Datei — vermutlich doppelte Zeile': 'duplicate address in the file — probably a duplicated row',
  'Adresse bereits im Verzeichnis': 'address already in the directory',
  'keine Zahl': 'not a number',
  'doppelt in der Datei': 'duplicate in the file',
  'bereits vergeben': 'already taken',
  'Pflichtfeld': 'required',
  'ungültiger Wert': 'invalid value',
  'Aktueller Wert (nicht in der Liste): {v}': 'Current value (not in the list): {v}',

  // Pagination (Users list)
  'Weiter': 'Next',
  'pro Seite': 'per page',
  '{total} Benutzer, Seite {page} von {totalPages}': '{total} users, page {page} of {totalPages}',
  'Insgesamt {n} Benutzer': '{n} users in total',

  // Test-user generator (bulk import wizard)
  'Datei hochladen': 'Upload a file',
  'Testbenutzer generieren': 'Generate test users',
  'Erzeugt eine Reihe synthetischer Testbenutzer (z. B. anna_m000, anna_m001, …) mit zufälligen Werten für die konfigurierten Zusatzattribute — nur für Tests/Demos.':
    'Creates a batch of synthetic test users (e.g. anna_m000, anna_m001, …) with randomised values for the configured extra attributes — for tests/demos only.',
  'Start': 'Start',
  'Anzahl': 'Count',
  'Mail-Domain (optional)': 'Mail domain (optional)',
  'Generieren und prüfen': 'Generate and review',
  'Vor- und Nachname für die Testbenutzer sind erforderlich.': 'First and last name are required for the test users.',
  'Anzahl muss zwischen 1 und 500 liegen.': 'Count must be between 1 and 500.',
  'Einheitliches Passwort für alle Testbenutzer verwenden': 'Use one password for all test users',
  'Bitte ein einheitliches Passwort eingeben oder eines vorschlagen lassen.': 'Please enter a shared password, or generate a suggestion.',
}

const tables = { de: {}, en }

export function t(key, params) {
  let s = (tables[i18n.lang] && tables[i18n.lang][key]) || key
  if (params) {
    for (const k of Object.keys(params)) {
      s = s.replaceAll('{' + k + '}', params[k])
    }
  }
  return s
}
