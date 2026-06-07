// Shared reactive session/meta state (Svelte 5 runes). Named `app` -- NOT
// `state` -- so the identifier does not shadow the `$state` rune.

export const app = $state({
  loading: true,
  reachable: true,
  provisioned: null, // null until known
  me: null,          // { uid, isAdmin, csrf } when logged in
  meta: null,        // server defaults for forms
  adminUid: 'admin',
  adminDn: '',       // resolved admin bind DN (must equal ldapd rootdn)
})
