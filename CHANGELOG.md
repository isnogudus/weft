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
- OpenBSD process sandboxing (`sandbox` / `chroot` / `user` / `group` options).
  After reading all files and opening the listener, weft confines itself with
  `pledge(2)`/`unveil(2)`; when started as root it also `chroot(2)`s (default
  `/var/empty`) and drops privileges to `_weft`. No-op on other platforms. The
  rc.d example now starts weft as root so it can drop privileges itself.

### Changed
- The ldapd TLS configuration (CA file / system trust store) is now loaded once
  at startup instead of per-connection, so no certificate file is read after the
  sandbox locks the filesystem.

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
