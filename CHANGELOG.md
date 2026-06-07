# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/), and the project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Support for connecting to ldapd over a local Unix socket via an `ldapi://`
  `ldap_url` (e.g. `ldapi:///var/run/ldapi`). For ldapi the connection is local
  and secured by filesystem permissions, so `tls_mode`, `ca_cert_file`,
  `insecure_skip_verify` and `allow_plain_bind` are ignored, and the
  network-plaintext warning is suppressed.
- Process sandboxing (`sandbox` / `chroot` / `user` / `group` options). After
  reading all files and opening the listener, when started as root weft
  `chroot(2)`s (default `/var/empty`) and drops privileges to `_weft` — on Linux,
  macOS, FreeBSD and the BSDs. On OpenBSD it additionally applies
  `pledge(2)`/`unveil(2)`. The chroot/privdrop is skipped when not root; no-op on
  non-Unix platforms. The rc.d example now starts weft as root so it can drop
  privileges itself.

- Privilege separation (`privsep` option, Unix). A privileged monitor process
  opens LDAP connections (DNS + connect, TCP or ldapi) and passes the connected
  descriptors to a re-exec'd, chrooted, unprivileged worker over a socketpair
  (`SCM_RIGHTS`). The worker keeps its `/var/empty` chroot even with a hostname
  or ldapi endpoint. On OpenBSD the monitor/worker are pledged to minimal
  promise sets (`…sendfd` / `…recvfd`). On by default (`privsep = true`); engages
  when started as root, so non-root and `-dev` run single-process. The monitor
  stops the worker by closing a shutdown pipe (no `kill`/`proc`); if the monitor
  dies the worker follows rather than being orphaned. `SIGHUP`/`SIGINT`/`SIGTERM`
  all trigger a clean shutdown.
- runit service example under `contrib/runit/` (foreground `run` + `svlogd`
  `log/run`). weft logs to stderr for the supervisor to capture.
- Optional syslog logging (`log = "syslog"`, `syslog_tag`). Under privsep the
  non-chrooted monitor owns the syslog connection (reconnecting across syslogd
  restarts, with an stderr fallback) and forwards the chrooted worker's log
  lines to it, so the worker never needs `/dev/log` in its chroot.

### Changed
- The ldapd TLS configuration (CA file / system trust store) is now loaded once
  at startup instead of per-connection, so no certificate file is read after the
  sandbox locks the filesystem.
- LDAP connections are now built from a raw transport connection (injected
  dialer) plus an explicit TLS step, instead of `ldap.DialURL`, so the same code
  serves both the default network dialer and the privsep fd-based dialer.

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

[0.1.0]: https://github.com/isnogudus/weft/releases/tag/v0.1.0
