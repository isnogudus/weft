# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and the project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

Upgrading: deployments behind relayd, httpd, nginx, a Kubernetes Ingress or
any other reverse proxy must now set `trusted_proxies` (e.g.
`trusted_proxies = ["127.0.0.1"]` for relayd on the same host) to keep
per-client login rate limiting. Without it weft no longer believes
`X-Forwarded-For` and keys the limit on the proxy's address, so all clients
share one bucket of five attempts per minute.

### Security
- **The login rate limit can no longer be bypassed with a forged
  `X-Forwarded-For`.** weft took the client IP from the header's first entry,
  which is whatever the client sent, so a fresh value per request bought a fresh
  bucket -- with or without a proxy. chi's `RealIP` middleware did the same with
  `X-Real-IP` and `True-Client-IP` and has been removed. The header is now only
  read when the connection comes from a trusted proxy, and then from the right:
  the client IP is the rightmost address that is not itself a trusted proxy.

### Added
- `trusted_proxies` option (default empty, `WEFT_TRUSTED_PROXIES`
  comma-separated): CIDRs or bare IPs of the reverse proxies whose
  `X-Forwarded-For` weft believes.

## [0.4.0] - 2026-10-09

Upgrading from 0.3.0: no configuration change is required, and nothing in the
directory has to be touched. The new `admin_can_change_own_password` defaults
to true, so the admin keeps "Mein Passwort". Passwords suggested by weft now
look like `k3pa.7xmq.e2tn` instead of German passphrases. Anything scripted
against `POST /api/me/password` as the admin should expect 409 (admin_dn is
the rootdn), 502 (written but unconfirmed) and, with the option off, 403.

### Added
- `admin_can_change_own_password` option (default true,
  `WEFT_ADMIN_CAN_CHANGE_OWN_PASSWORD`). Set false to hide "Mein Passwort" in
  the admin session and refuse the change with 403, e.g. when the admin is the
  rootdn and its password lives in the server configuration anyway.

### Changed
- **Generated passwords are random instead of German passphrases.**
  "Vorschlagen" and the bulk import now produce passwords like
  `k3pa.7xmq.e2tn`, the method of password-generator with lowercase and digits
  only: 12 characters, at least one of each, without `l`/`o` and `y`/`z`, in
  dot-separated blocks of four (60 bits, ample behind bcrypt). The passphrase
  word lists (~200 KB) are gone from the frontend.

### Fixed
- **The admin can change their own password again.** The change was written
  to `<user_id_attr>=<admin_uid>,ou=people,<base>` instead of the configured
  `admin_dn`, so with an admin entry outside `ou=people` (e.g.
  `cn=admin-dev,ou=admins,<base>`) it failed with "nicht gefunden". It now
  targets `admin_dn`, and only reports success once the new password actually
  binds. When `admin_dn` is the server's rootdn -- with no entry, or with one
  that slapd/ldapd ignore for the bind, as in `osixia/openldap` -- weft refuses
  with an explanation (change `rootpw` / `olcRootPW` on the server) and sets
  the entry back to the password it just bound with, instead of claiming a
  change that does not take effect and breaking the session. If the check
  fails for another reason (directory unreachable), weft keeps the change and
  reports it as unconfirmed rather than rolling back a change that may be in
  effect.

## [0.3.0] - 2026-08-20

Upgrading from 0.2.0: the setup wizard now runs *inside* an admin session --
log in with the rootpw first, then confirm the bootstrap. Anything scripted
against the JSON API needs adjusting: `POST /api/setup/bootstrap` requires an
admin session and no longer takes a password, and `GET /api/setup/status` no
longer reports `provisioned` (that moved to `needsSetup` on `/api/me`). No
configuration change is required, and nothing in the directory has to be
touched.

### Added
- **`GET /api/healthz`** — a real health endpoint: `200 {"status":"ok",
  "ldap":"up"}` while the directory server answers, `503` while it does not, so
  a supervisor can act on the status code alone. The body deliberately says no
  more than that (no admin DN, no LDAP address, no error text — the endpoint is
  unauthenticated; the reason is logged at `warn`). The probe connects and
  disconnects without binding or searching, so it needs no credentials and no
  anonymous access. Results are cached briefly and shared with
  `GET /api/setup/status`, so polling — and the SPA's own call on every page
  load — no longer costs one LDAP connection per request; concurrent probes
  coalesce into one dial. The Docker image gained a matching `HEALTHCHECK`, and
  `contrib/relayd.conf.example` carries the `check http "/api/healthz" code 200`
  variant, commented out with the single-host trade-off spelled out.
- **`log_level`** (`debug` | `info` | `warn` | `error`, default `info`; also
  `WEFT_LOG_LEVEL` / `-log-level`), independent of `log`, which keeps picking
  only the destination. `warn` switches the per-request access log off for a
  quiet production instance. `debug` adds one line per LDAP operation -- dial,
  bind DN, search base/filter/result count, add/modify/delete DN -- which is
  what makes a directory problem visible in weft's own log instead of only in
  the server's. Credentials never appear: `userPassword` is written pre-hashed
  and never logged, and a modify logs attribute names, not values. Towards
  syslog the level now also selects the severity (`LOG_DEBUG`/`LOG_INFO`/
  `LOG_WARNING`/`LOG_ERR`) rather than logging everything as INFO; on stderr,
  non-info lines carry a `debug: ` / `warning: ` / `error: ` prefix. Under
  privsep both processes resolve the same level, so the worker's LDAP debug
  lines appear in the same stream.

### Fixed
- **The setup wizard no longer loops on servers that deny anonymous access.**
  Whether the base structure exists was probed on an *unauthenticated*
  connection. OpenLDAP answers such a search by filtering the entry out rather
  than erroring, so with ACLs like the ones this README recommends
  (`by * none`), weft read a fully provisioned directory as a fresh one: the
  bootstrap succeeded, the following status check still said "not provisioned",
  and the wizard reappeared forever. The check now runs after login on the
  bound connection, where the rootdn bypasses ACLs.

### Changed
- **The setup wizard moved inside the admin session.** Log in as the admin uid
  first (the rootdn is synthetic, so this works on an empty directory); if the
  base structure is missing, the session shows the wizard, which bootstraps
  with the credentials it already holds instead of asking for the rootpw a
  second time. `POST /api/setup/bootstrap` now requires an admin session, so
  there is no longer an unauthenticated endpoint that confirms a guessed
  rootpw. `GET /api/setup/status` reports reachability only (a connect, no
  search, no bind) and no longer carries `provisioned`; `/api/me` gained
  `needsSetup`.
- With `allow_admin = false` the wizard is consequently unreachable — such a
  directory must be provisioned beforehand. Logged as a note at startup.

## [0.2.0] - 2026-06-07

### Added
- **Privilege separation** (Unix) is now the process model for every non-`-dev`
  run. A small privileged monitor opens LDAP connections (DNS + connect, TCP or
  the ldapi socket) and passes the connected descriptors to a re-exec'd,
  unprivileged worker over a `socketpair` (`SCM_RIGHTS`). Started as root the
  worker `chroot(2)`s to `/var/empty` and drops privileges to `_weft`; without
  root those steps are skipped but the same split applies. On OpenBSD the monitor
  and worker are confined with `pledge(2)`/`unveil(2)` to minimal promise sets and
  paths. The monitor stops the worker by closing a shutdown pipe (no `kill`/`proc`);
  if the monitor dies the worker follows rather than being orphaned.
  `SIGHUP`/`SIGINT`/`SIGTERM` all shut down cleanly. Controlled by `sandbox` /
  `chroot` / `user` / `group`. `-dev` and non-Unix platforms run single-process.
- Connect to ldapd over a local Unix socket via an `ldapi://` `ldap_url` (e.g.
  `ldapi:///var/run/ldapi`). The connection is local and secured by filesystem
  permissions, so `tls_mode` / `ca_cert_file` / `insecure_skip_verify` /
  `allow_plain_bind` are ignored.
- Optional syslog logging (`log = "syslog"`, `syslog_tag`). Under privsep the
  non-chrooted monitor owns the syslog connection (reconnecting across syslogd
  restarts, with an stderr fallback) and forwards the chrooted worker's log lines,
  so the worker never needs `/dev/log` in its chroot.
- `allow_admin` option (default true). Set false for a self-service-only
  deployment: the admin uid is rejected at login, so no admin/management UI is
  reachable. The active mode is logged at startup ("admin login: ENABLED/DISABLED").
- Idle auto-logout: the server expires sessions after `session_timeout` of
  inactivity (sliding); the SPA now switches to the login view on expiry.
- `-insecure` flag / `insecure_skip_verify` to skip LDAP TLS verification for a
  self-signed server (with a startup warning).
- runit service example under `contrib/runit/` (foreground `run` + `svlogd`
  `log/run`).

### Changed
- The ldapd TLS configuration (CA file / system trust store) is loaded once at
  startup; LDAP connections are built from an injected raw dialer plus an explicit
  TLS step (instead of `ldap.DialURL`), shared by the default network dialer and
  the privsep fd-based dialer.

## [0.1.0] - 2026-06-07

First public release.

### Added
- Single static binary: Go backend serving an embedded Svelte 5 SPA, with a
  JSON API. Cross-compiles to `openbsd/amd64`.
- Passthrough-bind authentication against an external LDAP server (target:
  OpenBSD `ldapd`). No service account; sessions re-bind as the logged-in user.
- Admin = the ldapd `rootdn`. Configurable admin bind DN (`admin_uid` /
  `admin_dn`), logged at startup and shown in the setup wizard.
- Opinionated directory layout: `ou=people` (RDN `uid`), `ou=groups`
  (posixGroup only), a shared default primary group.
- Users with optional POSIX and Mail profiles; collision-safe uid/gid
  auto-allocation with admin override; bcrypt `{CRYPT}` passwords.
- Groups: create/delete, member management, effective-group view (primary via
  gidNumber + supplementary via memberUid).
- uid rename via add → memberUid fix-up → delete (ldapd has no ModifyDN).
- Setup wizard that provisions the base entry, OUs and default group on an empty
  directory (idempotent).
- Self-service: view own profile/groups, change own password.
- Read-only user detail view (click a row); admin can jump to editing.
- Bilingual UI (German/English) with a runtime toggle, persisted per browser.
- Security: server-side sessions, `HttpOnly`/`Secure`/`SameSite=Strict` cookies,
  CSRF synchronizer token, login rate limit, TLS to the LDAP server, optional
  `-insecure` for self-signed certs, request logging.
- Docs and OpenBSD operational examples: `weft.toml`, `ldapd.conf` (schema +
  ACLs), `rc.d` service, `relayd` TLS termination.

[Unreleased]: https://github.com/isnogudus/weft/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/isnogudus/weft/releases/tag/v0.4.0
[0.3.0]: https://github.com/isnogudus/weft/releases/tag/v0.3.0
[0.2.0]: https://github.com/isnogudus/weft/releases/tag/v0.2.0
[0.1.0]: https://github.com/isnogudus/weft/releases/tag/v0.1.0
