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
