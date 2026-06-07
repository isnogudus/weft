# weft

[![CI](https://github.com/isnogudus/weft/actions/workflows/ci.yml/badge.svg)](https://github.com/isnogudus/weft/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/isnogudus/weft)](https://github.com/isnogudus/weft/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A small, opinionated web UI to administer users and groups in an **existing,
external LDAP server** — primarily OpenBSD [`ldapd(8)`](https://man.openbsd.org/ldapd.8).
weft does *not* embed a directory; it is a thin, single-binary admin front-end
that authenticates **through the LDAP itself** (passthrough bind).

The UI is bilingual (German/English, switchable at runtime); the code,
identifiers and API are English.

## Highlights

- **Single static binary.** The Svelte SPA is built and embedded via `embed.FS`;
  there is no Node runtime at runtime. Cross-compiles cleanly to `openbsd/amd64`.
- **Minimal config.** At its core just `ldap_url` + `base_dn` + a TLS mode.
- **No service account.** Every session re-binds as the logged-in user; the
  directory's own ACLs do the enforcing.
- **Opinionated structure.** `ou=people` (RDN `uid`), `ou=groups` (posixGroup
  only), a shared default primary group, bcrypt `{CRYPT}` passwords.
- **Bilingual UI.** German/English, toggled in the header (DE/EN) and remembered
  per browser; defaults to the browser language.
- **Process sandboxing.** After reading its files, when started as root weft
  `chroot(2)`s and drops privileges to `_weft` (Linux, macOS, FreeBSD, the BSDs);
  on OpenBSD it additionally applies `pledge(2)`/`unveil(2)`.

## Authorization model (read this)

ldapd's ACLs **cannot express group membership** — a rule's subject is only
`by any`, `by self`, or `by <single dn>`. weft therefore uses the simplest model
ldapd can enforce honestly:

- **Admin = the ldapd `rootdn`.** You log in by typing the admin uid
  (`admin_uid`, default `admin`); the session then binds as the admin DN, which
  must equal ldapd's `rootdn`. The bind DN is `admin_dn` if set, otherwise the
  derived `uid=<admin_uid>,ou=people,<base>`. weft **prints the resolved admin
  bind DN at startup** and shows it in the setup wizard so you can match it to
  `rootdn`. The rootdn bypasses ACLs, so it can create/modify everything; it is
  synthetic and need not exist as an entry.
- **Everyone else = self-service only.** They may view their own profile/groups
  and change their own password (`by self` write, restricted to `userPassword`).

If you need multiple delegated admins or group-based ACLs, use OpenLDAP instead.

## Directory layout weft manages

```
dc=example,dc=org
├── ou=people            users, RDN uid
│   └── uid=alice        inetOrgPerson (+ posixAccount, + mail — optional)
└── ou=groups            groups, posixGroup only
    └── cn=users         the default primary group (gidNumber on each user)
```

- **User** = `inetOrgPerson` base, with optional **POSIX** (`posixAccount`:
  uidNumber/gidNumber/homeDirectory/loginShell/gecos — required for shell
  accounts) and optional **Mail** (`mail` + aliases) profiles.
- **Group** = `posixGroup` with `cn`, `gidNumber`, `memberUid` (uid-based, may be
  empty). Primary group is the user's `gidNumber`; supplementary groups are
  `memberUid` entries. weft merges both for the effective view.
- **uid/gid numbers** are auto-allocated (smallest free in the configured range,
  serialised by an app-side lock) and can be overridden by the admin.

## Verified ldapd facts

These shaped the design (checked against the ldapd source and man pages):

| Fact | Consequence in weft |
|------|---------------------|
| Ships `core`, `inetorgperson`, `nis` schemas and **enforces** loaded schema | Entries carry the full objectClass chain and all MUST attributes |
| **No ModifyDN/ModRDN** operation | uid rename is done as *add-new → fix memberUid → delete-old* (not atomic) |
| Bind verifies `{CRYPT}` (= bcrypt on OpenBSD), `{SHA}`, `{SSHA}` | weft writes `userPassword: {CRYPT}$2b$...`; it never reads or verifies hashes itself |
| ACL subjects are only `any` / `self` / a single DN | Admin = rootdn; users limited to `by self` userPassword writes |
| The namespace suffix entry is **not** auto-created | the setup wizard creates the base entry (`dc=`/`o=`/`ou=`) before the OUs, so the directory may start empty |

## Build

Requires Go (see the version in [`go.mod`](go.mod)) and Node ≥ 20 — both only on
the build host. The resulting binary has no runtime dependencies.

```sh
make build            # builds the SPA, then the host binary -> ./weft
make build-openbsd    # cross-compiles a static binary -> ./weft.openbsd-amd64
make test             # Go tests (no LDAP needed; uses the in-memory Fake)
```

`make web` builds just the frontend into `web/dist` (embedded by `web/embed.go`).
The frontend must be built before the Go binary, because the SPA is embedded via
`embed.FS` — the `make` targets handle that ordering for you. A placeholder
`web/dist/index.html` is committed so `go test`/`go vet` work without Node.

### Cross-compiling for OpenBSD

The whole point is to develop on any platform and ship one static binary to
OpenBSD. The easiest way:

```sh
make build-openbsd        # -> ./weft.openbsd-amd64
```

That target first builds the SPA, then runs, in effect:

```sh
GOOS=openbsd GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags '-s -w -X main.version=<version>' \
  -o weft.openbsd-amd64 ./cmd/weft
```

Notes:

- `CGO_ENABLED=0` keeps it pure Go — no libc linkage, no cross C toolchain
  needed. The binary is self-contained (it still uses OpenBSD's `ld.so`, which is
  always present).
- `<version>` comes from `git describe` via the Makefile (e.g. `v0.1.0`); it is
  reported by `weft -version`.
- For the arm64 port, set `GOARCH=arm64` instead.
- No Go, Node or other tooling is required **on the OpenBSD target** — only the
  binary and a config file.

Copy the artifact over and install it (see [Deploy on OpenBSD](#deploy-on-openbsd)):

```sh
scp weft.openbsd-amd64 you@host:/tmp/weft
# on the host:
doas install -o root -g bin -m 0555 /tmp/weft /usr/local/bin/weft
```

## Configure & run

Copy [`weft.toml.example`](weft.toml.example) to `weft.toml` and edit. Then:

```sh
./weft -config weft.toml
```

Config precedence: defaults < TOML file < `WEFT_*` env vars < flags. Flags:
`-config`, `-listen`, `-insecure` (skip LDAP TLS certificate verification —
prefer `ca_cert_file`), `-dev`, `-version`. Key env overrides: `WEFT_LDAP_URL`,
`WEFT_BASE_DN`, `WEFT_ADMIN_UID`, `WEFT_ADMIN_DN`, `WEFT_LISTEN_ADDR`,
`WEFT_TLS_MODE`, `WEFT_SESSION_TIMEOUT`, `WEFT_INSECURE_SKIP_VERIFY`.

For a same-host deployment, point `ldap_url` at ldapd's Unix socket —
`ldap_url = "ldapi:///var/run/ldapi"` (with `listen on "/var/run/ldapi"` in
`ldapd.conf`). With an `ldapi://` url the connection is local and secured by
filesystem permissions, so `tls_mode`, `ca_cert_file`, `insecure_skip_verify`
and `allow_plain_bind` are all ignored — no TLS or certificates needed.

At startup weft logs the LDAP server URL, the resolved admin bind DN, and (when
enabled) warnings for `insecure_skip_verify` / `tls_mode=plain` (suppressed for
`ldapi://`). Every HTTP
request is logged as `METHOD /path -> status (duration)` (never bodies or
credentials); unknown `/api` paths return a JSON `{"error":"unbekannter
API-Endpunkt: …"}` so they are distinguishable from a proxy's 404.

### First-run setup

On first start weft checks whether `ou=people` exists. If not, the UI shows a
**setup wizard**: enter the ldapd **rootpw** and weft binds once as the rootdn to
create the base/suffix entry, `ou=people`, `ou=groups`, and the default `users`
group. Afterwards log in as the admin uid (e.g. `admin`) with the rootpw. The
wizard is idempotent, so it is safe to re-run.

The admin bind DN that weft uses (logged at startup, shown in the wizard) must
equal ldapd's `rootdn`. If your `rootdn` is not `uid=<admin_uid>,ou=people,<base>`,
set `admin_dn` in `weft.toml` to match it exactly.

### Local development

```sh
./weft -dev -listen 127.0.0.1:8099      # in-memory fake directory, no LDAP
cd web && npm run dev                    # Vite dev server, proxies /api to :8099
```

In `-dev` mode the admin is `admin` / `rootpw` (override with `-dev-rootpw`).

## Security

- TLS to the LDAP server is enforced whenever credentials are sent (`plain`
  requires an explicit dev opt-in).
- Sessions are server-side; the opaque id is an `HttpOnly; Secure; SameSite=Strict`
  cookie. Bind credentials live only in server memory, never in the cookie, never
  logged.
- CSRF: a synchronizer token (returned by `/login` and `/me`, echoed in the
  `X-CSRF-Token` header) is required on all state-changing requests.
- Login is rate-limited per client IP.
- Passwords are hashed client-side (bcrypt) before `userPassword` is written;
  inputs longer than 72 bytes are rejected (bcrypt truncation).
- Certificate verification can be skipped for a self-signed LDAP server via
  `insecure_skip_verify` / `-insecure` (a startup warning is logged); prefer
  pinning the CA with `ca_cert_file`.

### Sandboxing

weft confines itself **after** reading every file it needs (config, CA bundle /
system trust store, TLS keypair) and opening its listening socket — this is the
last step before serving, so nothing root-owned is read afterwards.

- **Cross-platform (Linux, macOS, FreeBSD, the BSDs):** when **started as root**
  weft `chroot(2)`s to `chroot` (default `/var/empty`) and drops privileges to
  `user`/`group` (default `_weft`). If not started as root, this step is skipped.
- **OpenBSD additionally:** `pledge(2)` reduces the permitted syscalls to roughly
  `stdio rpath inet` (plus `dns` for hostname resolution, `unix` for an ldapi
  socket), and `unveil(2)` restricts (or, under chroot, removes) filesystem
  access.

This is why the rc.d script starts weft as root (it drops privileges itself —
see [`contrib/weft.rc`](contrib/weft.rc)). Everything is controlled by the
`sandbox` / `chroot` / `user` / `group` options; on non-Unix platforms it is a
no-op.

Caveat — single-process mode only (i.e. `privsep = false`): a chroot to
`/var/empty` cannot reach DNS config or Unix sockets. For a chrooted deployment
use `ldaps://<IP>` with a `ca_cert_file`; with an `ldapi://` url weft **skips the
chroot automatically** while still dropping privileges and applying
`pledge`/`unveil`. The **default privsep mode** (below) removes this caveat
entirely — the chroot holds for hostnames and ldapi alike.

### Privilege separation (privsep)

privsep is **on by default** (`privsep = true`, Unix). It engages when weft is
started as root (so the worker can chroot and drop privileges); non-root and
`-dev` run single-process. Set `privsep = false` to disable it. It runs weft as
two processes, in the style of OpenBSD daemons:

- A small **privileged monitor** opens connections to the LDAP server — DNS +
  `connect`, for TCP *or* the ldapi Unix socket — and passes the connected file
  descriptors to the worker over a `socketpair` using `SCM_RIGHTS`. It parses no
  request data. On OpenBSD it is pledged to `stdio rpath inet dns unix sendfd`.
  It stops the worker by closing a shutdown pipe (not `kill(2)`), so it needs no
  `proc` promise; if the monitor dies the pipe closes too, so the worker is never
  orphaned.
- An unprivileged **worker** (re-exec'd, `chroot`ed to `/var/empty`, dropped to
  `_weft`) serves HTTP and the JSON API and speaks LDAP over the descriptors it
  receives. It never needs DNS, the ldapi socket, or any filesystem, so the
  `/var/empty` chroot holds for **every** transport — hostname or ldapi included.
  On OpenBSD it is pledged to `stdio inet recvfd`.

Because the monitor re-dials on demand, idle-dropped connections recover (unlike
a static pre-opened pool). fd passing is a portable Unix mechanism, so privsep
works on Linux/macOS/FreeBSD too — only the `pledge`/`unveil` layer is
OpenBSD-specific.

## Deploy on OpenBSD

1. Configure `ldapd` — see [`contrib/ldapd.conf.example`](contrib/ldapd.conf.example)
   for the schema includes, the `rootdn`/`rootpw`, and the exact ACLs (hide
   `userPassword` on read, allow read, allow bind, allow `by self` password
   write). Reload with `rcctl reload ldapd`.
2. Create the service user and install the binary/config. weft starts as root
   and drops to `_weft` itself, so the config is owned by root:
   ```sh
   useradd -d /var/empty -s /sbin/nologin -L daemon _weft
   install -o root -g bin  -m 0555 weft.openbsd-amd64 /usr/local/bin/weft
   install -o root -g wheel -m 0600 weft.toml /etc/weft.toml
   ```
3. Install the rc.d script [`contrib/weft.rc`](contrib/weft.rc) as
   `/etc/rc.d/weft`, then `rcctl enable weft && rcctl start weft`. weft chroots
   to `/var/empty` and drops to `_weft` after startup (see
   [Sandboxing](#sandboxing); set `sandbox=false` to opt out).
4. Terminate TLS in front of weft with `relayd` (or `httpd`) — see
   [`contrib/relayd.conf.example`](contrib/relayd.conf.example). weft listens on
   `127.0.0.1:8080`; the proxy should forward the real client IP via
   `X-Forwarded-For` so the login rate limit keys correctly. (For a standalone
   setup without a proxy, set `tls_cert_file`/`tls_key_file` in `weft.toml`.)

Under a supervisor instead of rc.d — e.g. **runit** — use
[`contrib/runit/`](contrib/runit/): a `run` script that `exec`s weft as root in
the foreground (weft re-execs the chrooted worker itself), plus a `log/run` that
captures the logs with `svlogd`.

## Logging

weft logs to **stderr** (Go's standard logger — no syslog, no log file of its
own), one line per request and for lifecycle events, so the process supervisor
owns log capture and rotation:

- **runit:** the example `run` does `exec 2>&1` and the `log/run` service runs
  `svlogd -tt /var/log/weft` → timestamped, rotated logs in `/var/log/weft`.
- **OpenBSD `rc.d`:** stderr is not captured by default; redirect it in the rc
  script or run weft under a supervisor. (Native syslog support can be added if
  wanted — open an issue.)

Credentials and password material are never logged. With privsep, both the
monitor and the worker write to the same inherited stderr, so all logs appear in
one stream.

## API sketch

All under `/api`, JSON. Writes require the CSRF header.

```
POST /login            POST /logout           GET /me
GET  /me/profile       GET  /me/groups        POST /me/password
GET  /setup/status     POST /setup/bootstrap  GET /meta
GET/POST /users        GET/PUT/DELETE /users/{uid}
POST /users/{uid}/password    POST /users/{uid}/rename    GET /users/{uid}/groups
GET/POST /groups       DELETE /groups/{cn}
POST /groups/{cn}/members     DELETE /groups/{cn}/members/{uid}
```

`/users*` and `/groups*` are admin-only; `/me*` is available to every
authenticated user.

## Project layout

```
cmd/weft            main: flags, config, wiring, HTTP server
internal/config     TOML + env + flags, defaults, DN templates
internal/directory  the Directory/Conn abstraction + sentinel errors
  ├── ldapd         go-ldap/v3 implementation against ldapd
  └── fake          in-memory implementation for tests and -dev
internal/idalloc    pure next-free-number allocation
internal/password   bcrypt -> {CRYPT}
internal/service    validation, hashing, id allocation, bootstrap
internal/server     sessions, CSRF, rate limit, JSON handlers, SPA serving
web                 Svelte 5 SPA (Vite) + embed.go
contrib             ldapd.conf / relayd.conf / rc.d examples
```

## Testing

`make test` runs everything against the Fake directory — no OpenBSD or ldapd
required. The Fake mirrors ldapd's authorization (admin writes all, users write
only their own entry) and verifies binds against the stored bcrypt hash, so the
HTTP integration tests exercise realistic auth flows. For true integration
testing, ldapd is OpenBSD-only; run a VM (e.g. via `vmd`) or test against the
binary in an OpenBSD CI runner.

## License

MIT — see [LICENSE](LICENSE).
