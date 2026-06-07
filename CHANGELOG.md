# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and the project follows
[Semantic Versioning](https://semver.org/).

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

[0.2.0]: https://github.com/isnogudus/weft/releases/tag/v0.2.0
[0.1.0]: https://github.com/isnogudus/weft/releases/tag/v0.1.0
