// Package config loads weft's configuration from (in increasing precedence)
// built-in defaults, a TOML file, environment variables (WEFT_*) and command
// line flags. The core is deliberately minimal -- ldap_url + base_dn -- with
// good defaults for everything else.
package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"weft/internal/idalloc"
)

// TLSMode selects how weft connects to the LDAP server.
type TLSMode string

const (
	TLSLDAPS    TLSMode = "ldaps"    // implicit TLS (ldaps://, port 636)
	TLSStartTLS TLSMode = "starttls" // ldap:// then StartTLS
	TLSPlain    TLSMode = "plain"    // no TLS -- dev only, must be explicitly allowed
)

// Directory server flavors.
const (
	DirectoryLdapd    = "ldapd"    // OpenBSD ldapd(8), the original target
	DirectoryOpenLDAP = "openldap" // OpenLDAP slapd
)

// Config is the fully-resolved configuration.
type Config struct {
	// Core
	LDAPURL string `toml:"ldap_url"`
	BaseDN  string `toml:"base_dn"`

	// Directory selects the server flavor ("ldapd" or "openldap"). The wire
	// protocol is the same; the flavor decides server-specific behaviour such
	// as how a uid rename is performed (OpenLDAP: ModifyDN; ldapd: copy+delete).
	Directory string `toml:"directory"`

	// UserIDAttr selects which LDAP attribute is the user's naming/identifier
	// attribute: "uid" (default) or "cn". It becomes the RDN in every user DN
	// (UserDN), the login name, and the value tracked in group memberUid.
	//
	// Set "cn" for directories that name user entries by cn= instead of uid=
	// (e.g. some Matrix/Synapse LDAP setups). The identifier keeps the same
	// strict charset either way (see service.ValidName) -- Synapse's own
	// localpart rules reject spaces/uppercase/non-ASCII, so cn must hold a
	// short login handle here, not a full display name, exactly like uid
	// would. When UserIDAttr is "cn", the entry's cn attribute IS the
	// identifier (LDAP requires the RDN's value to be one of the entry's own
	// attribute values), so there is no separately editable "cn" display
	// field in this mode -- givenName/sn/displayName still are.
	UserIDAttr string `toml:"user_id_attr"`

	// Transport security to the LDAP server.
	TLSMode            TLSMode `toml:"tls_mode"`
	CACertFile         string  `toml:"ca_cert_file"`
	InsecureSkipVerify bool    `toml:"insecure_skip_verify"` // skip LDAP cert verification (self-signed); logs a warning
	AllowPlainBind     bool    `toml:"allow_plain_bind"`     // dev only

	// Admin identity. The admin logs in by typing AdminUID as the username; the
	// session then binds as the admin DN, which must equal ldapd's rootdn.
	//
	// AdminDN is that bind DN. Leave it empty to derive the default
	// <user_id_attr>=<AdminUID>,ou=people,<base>; set it explicitly when your
	// rootdn has a different shape (e.g. "cn=admin,dc=example,dc=org"). The admin is
	// synthetic -- it need not exist as a directory entry (ldapd special-cases
	// the rootpw). Resolve via AdminBindDN().
	AdminUID string `toml:"admin_uid"`
	AdminDN  string `toml:"admin_dn"`

	// AllowAdmin controls whether the admin may log in to the web UI. Set false
	// for a self-service-only deployment: the admin uid is rejected at login, so
	// no admin session (and no management UI) is ever reachable. The rootdn can
	// still be used out-of-band (ldapd) and for the one-time setup wizard.
	AllowAdmin bool `toml:"allow_admin"`

	// Directory layout (good defaults; rarely changed).
	PeopleOU     string `toml:"people_ou"`
	GroupsOU     string `toml:"groups_ou"`
	PrimaryGroup string `toml:"primary_group"` // shared default primary group

	// POSIX defaults.
	UIDMin       int    `toml:"uid_min"`
	UIDMax       int    `toml:"uid_max"`
	GIDMin       int    `toml:"gid_min"`
	GIDMax       int    `toml:"gid_max"`
	HomeTemplate string `toml:"home_template"` // e.g. "/home/{uid}"
	DefaultShell string `toml:"default_shell"` // OpenBSD default: /bin/ksh

	// Mail attribute mapping. Aliases live in MailAliasAttr; when empty they are
	// stored as additional values of MailAttr.
	MailAttr      string `toml:"mail_attr"`
	MailAliasAttr string `toml:"mail_alias_attr"`

	// Extra user attributes managed by weft beyond the built-in set, defined as
	// [[user_attr]] tables. Attributes outside the inetOrgPerson chain (e.g. "c"
	// or site-specific schema) additionally need their auxiliary objectClass
	// listed in UserExtraClasses, and the schema loaded on the server.
	UserAttrs        []UserAttr `toml:"user_attr"`
	UserExtraClasses []string   `toml:"user_extra_classes"`

	// TestUserGenerator shows a "generate test users" option in the bulk
	// import wizard: given a first/last name and a count, it creates a block
	// of synthetic users (name_a000, name_a001, ...) with randomised values
	// for whichever extra attributes are configured, for load-testing or demo
	// data. Off by default -- this is a convenience for admins who already
	// have that access, not a new capability, but its blast radius (bulk
	// account creation) warrants an explicit opt-in.
	TestUserGenerator bool `toml:"enable_test_user_generator"`

	// Password hashing.
	BcryptCost        int `toml:"bcrypt_cost"`
	MaxPasswordLength int `toml:"max_password_length"` // bcrypt truncates at 72 bytes

	// Sandbox (OpenBSD only; no-op elsewhere). After reading all files weft
	// confines itself with pledge(2)/unveil(2), and — when started as root —
	// chroot(2)s and drops privileges to User. Chroot is skipped if not root.
	// (Confinement applies inside the privilege-separated monitor/worker, which
	// is the model for every non-dev run on Unix.)
	Sandbox bool   `toml:"sandbox"` // master switch (default true)
	Chroot  string `toml:"chroot"`  // chroot dir when root (default /var/empty); empty disables chroot
	User    string `toml:"user"`    // drop to this user when chrooting (default _weft)
	Group   string `toml:"group"`   // drop to this group ("" = the user's primary group)

	// Logging. "stderr" (default) lets the supervisor capture logs; "syslog"
	// writes to the local syslog. Under privsep the monitor owns the syslog
	// connection (and reconnects across syslogd restarts) and forwards the
	// chrooted worker's log output to it.
	Log       string `toml:"log"`        // "stderr" | "syslog"
	SyslogTag string `toml:"syslog_tag"` // syslog program tag (default "weft")

	// HTTP server.
	ListenAddr     string   `toml:"listen_addr"`
	TLSCertFile    string   `toml:"tls_cert_file"` // optional standalone TLS
	TLSKeyFile     string   `toml:"tls_key_file"`
	SessionTimeout Duration `toml:"session_timeout"` // e.g. "30m"
	CookieSecure   bool     `toml:"cookie_secure"`   // set false only for local http dev
}

// UserAttr defines one configurable extra user attribute (a [[user_attr]]
// table). Attr is the LDAP attribute name; the labels are shown in the UI.
// Options, if non-empty, restricts the value to a fixed set (rendered as a
// dropdown instead of free text) -- e.g. a tri-state visibility flag.
type UserAttr struct {
	Attr     string           `toml:"attr"`
	LabelDE  string           `toml:"label_de"`
	LabelEN  string           `toml:"label_en"`
	Required bool             `toml:"required"`
	Options  []UserAttrOption `toml:"options"`
}

// UserAttrOption is one selectable value of an enum-constrained UserAttr (a
// nested [[user_attr.options]] table). Value is the literal string written to
// LDAP; an empty Value clears the attribute (lets an admin pick a "default"
// entry that leaves the attribute unset).
type UserAttrOption struct {
	Value   string `toml:"value"`
	LabelDE string `toml:"label_de"`
	LabelEN string `toml:"label_en"`
}

// Label returns the UI label for the given language, falling back to the other
// language and finally the raw attribute name.
func (a UserAttr) Label(lang string) string {
	return pickLabel(lang, a.LabelDE, a.LabelEN, a.Attr)
}

// HasOption reports whether v is one of the attribute's configured Options
// (exact match; only meaningful when Options is non-empty).
func (a UserAttr) HasOption(v string) bool {
	for _, o := range a.Options {
		if o.Value == v {
			return true
		}
	}
	return false
}

// Label returns the UI label for the given language, falling back to the other
// language and finally the raw stored value.
func (o UserAttrOption) Label(lang string) string {
	return pickLabel(lang, o.LabelDE, o.LabelEN, o.Value)
}

func pickLabel(lang, de, en, fallback string) string {
	if lang == "de" && de != "" {
		return de
	}
	if en != "" {
		return en
	}
	if de != "" {
		return de
	}
	return fallback
}

// Duration is a time.Duration that decodes from a TOML/env string like "30m".
type Duration time.Duration

// D returns the underlying time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalText implements encoding.TextUnmarshaler for TOML decoding.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Default returns a Config populated with sensible defaults. The core fields
// (LDAPURL, BaseDN, AdminDN) are intentionally left empty and must be supplied.
func Default() Config {
	return Config{
		Directory:         DirectoryLdapd,
		UserIDAttr:        "uid",
		TLSMode:           TLSLDAPS,
		AdminUID:          "admin",
		AllowAdmin:        true,
		PeopleOU:          "people",
		GroupsOU:          "groups",
		PrimaryGroup:      "users",
		UIDMin:            10000,
		UIDMax:            59999,
		GIDMin:            10000,
		GIDMax:            59999,
		HomeTemplate:      "/home/{uid}",
		DefaultShell:      "/bin/ksh",
		MailAttr:          "mail",
		MailAliasAttr:     "",
		BcryptCost:        12,
		MaxPasswordLength: 72,
		Sandbox:           true,
		Log:               "stderr",
		SyslogTag:         "weft",
		Chroot:            "/var/empty",
		User:              "_weft",
		ListenAddr:        "127.0.0.1:8080",
		SessionTimeout:    Duration(30 * time.Minute),
		CookieSecure:      true,
	}
}

// UIDRange / GIDRange expose the allocation windows to the directory layer.
func (c Config) UIDRange() idalloc.Range { return idalloc.Range{Min: c.UIDMin, Max: c.UIDMax} }
func (c Config) GIDRange() idalloc.Range { return idalloc.Range{Min: c.GIDMin, Max: c.GIDMax} }

// PeopleDN returns the people OU DN, e.g. "ou=people,dc=example,dc=org".
func (c Config) PeopleDN() string { return "ou=" + c.PeopleOU + "," + c.BaseDN }

// GroupsDN returns the groups OU DN.
func (c Config) GroupsDN() string { return "ou=" + c.GroupsOU + "," + c.BaseDN }

// UserRDN builds just the RDN ("<user_id_attr>=<value>") for an identifier,
// e.g. for a ModifyDN request's new RDN.
func (c Config) UserRDN(id string) string {
	return c.UserIDAttr + "=" + escapeDNValue(id)
}

// UserDN builds the bind/entry DN for an identifier value, using the
// configured UserIDAttr ("uid" or "cn") as the RDN attribute.
func (c Config) UserDN(id string) string {
	return c.UserRDN(id) + ",ou=" + c.PeopleOU + "," + c.BaseDN
}

// GroupDN builds the entry DN for a group cn.
func (c Config) GroupDN(cn string) string {
	return "cn=" + escapeDNValue(cn) + ",ou=" + c.GroupsOU + "," + c.BaseDN
}

// escapeDNValue escapes a value for safe use in an RFC 4514 DN string:
// leading space/'#', trailing space, and the characters , + " \ < > ; are
// backslash-escaped; NUL is rejected outright (ValidName already excludes all
// of these, so this is defense in depth against DN injection, not the primary
// guard).
func escapeDNValue(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r == 0:
			continue // NUL has no valid escape a directory server will accept; drop it
		case r == ' ' && (i == 0 || i == len(runes)-1):
			b.WriteByte('\\')
			b.WriteRune(r)
		case strings.ContainsRune(`,+"\<>;`, r):
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '#' && i == 0:
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HomeDir renders the home directory for a uid from HomeTemplate.
func (c Config) HomeDir(uid string) string {
	return strings.ReplaceAll(c.HomeTemplate, "{uid}", uid)
}

// AdminBindDN returns the admin's bind DN: the explicit AdminDN if set,
// otherwise the derived uid=<AdminUID>,ou=people,<base>. It must equal the
// rootdn configured in ldapd.conf.
func (c Config) AdminBindDN() string {
	if c.AdminDN != "" {
		return c.AdminDN
	}
	return c.UserDN(c.AdminUID)
}

// IsAdminUID reports whether the given uid is the admin (case-insensitive).
func (c Config) IsAdminUID(uid string) bool {
	return c.AdminUID != "" && strings.EqualFold(uid, c.AdminUID)
}

// IsLDAPI reports whether ldap_url uses the ldapi:// scheme (a local Unix-domain
// socket, e.g. "ldapi:///var/run/ldapi"). For ldapi the connection is local and
// secured by filesystem permissions, so TLS and allow_plain_bind do not apply
// and are ignored.
func (c Config) IsLDAPI() bool {
	u, err := url.Parse(c.LDAPURL)
	return err == nil && strings.EqualFold(u.Scheme, "ldapi")
}

// LDAPISocketPath returns the Unix socket path for an ldapi:// url, else "".
func (c Config) LDAPISocketPath() string {
	u, err := url.Parse(c.LDAPURL)
	if err != nil || !strings.EqualFold(u.Scheme, "ldapi") {
		return ""
	}
	if u.Path != "" {
		return u.Path
	}
	return "/var/run/ldapi"
}

// DialTarget returns the (network, address) for a raw connection to the LDAP
// server: ("unix", socketpath) for ldapi, otherwise ("tcp", host:port) with the
// scheme's default port filled in. Used by the default dialer and by the
// privsep monitor.
func (c Config) DialTarget() (network, address string, err error) {
	if c.IsLDAPI() {
		return "unix", c.LDAPISocketPath(), nil
	}
	u, err := url.Parse(c.LDAPURL)
	if err != nil {
		return "", "", fmt.Errorf("config: invalid ldap_url: %w", err)
	}
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "ldaps") {
			port = "636"
		} else {
			port = "389"
		}
	}
	return "tcp", net.JoinHostPort(u.Hostname(), port), nil
}

// LDAPHostIsName reports whether the LDAP host is a DNS name (not an IP and not
// the ldapi socket), i.e. whether resolving it requires DNS at runtime.
func (c Config) LDAPHostIsName() bool {
	if c.IsLDAPI() {
		return false
	}
	u, err := url.Parse(c.LDAPURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host != "" && net.ParseIP(host) == nil
}

// Validate checks the resolved configuration for consistency.
func (c Config) Validate() error {
	if c.LDAPURL == "" {
		return fmt.Errorf("config: ldap_url is required")
	}
	u, err := url.Parse(c.LDAPURL)
	if err != nil {
		return fmt.Errorf("config: invalid ldap_url: %w", err)
	}
	// ldapi:// is a local Unix socket: it overrides and ignores tls_mode and
	// allow_plain_bind (there is no transport to secure).
	if !c.IsLDAPI() {
		switch c.TLSMode {
		case TLSLDAPS, TLSStartTLS:
		case TLSPlain:
			if !c.AllowPlainBind {
				return fmt.Errorf("config: tls_mode=plain requires allow_plain_bind=true (dev only)")
			}
		default:
			return fmt.Errorf("config: invalid tls_mode %q", c.TLSMode)
		}
		if c.TLSMode == TLSLDAPS && u.Scheme != "ldaps" {
			return fmt.Errorf("config: tls_mode=ldaps but ldap_url scheme is %q", u.Scheme)
		}
	}
	if c.BaseDN == "" {
		return fmt.Errorf("config: base_dn is required")
	}
	if c.Directory != DirectoryLdapd && c.Directory != DirectoryOpenLDAP {
		return fmt.Errorf("config: directory must be %q or %q", DirectoryLdapd, DirectoryOpenLDAP)
	}
	if c.UserIDAttr != "uid" && c.UserIDAttr != "cn" {
		return fmt.Errorf("config: user_id_attr must be %q or %q", "uid", "cn")
	}
	if c.AdminUID == "" {
		return fmt.Errorf("config: admin_uid is required")
	}
	if !c.UIDRange().Valid() || !c.GIDRange().Valid() {
		return fmt.Errorf("config: invalid uid/gid range")
	}
	if c.BcryptCost < 4 || c.BcryptCost > 31 {
		return fmt.Errorf("config: bcrypt_cost out of range (4..31)")
	}
	if c.Log != "stderr" && c.Log != "syslog" {
		return fmt.Errorf("config: log must be \"stderr\" or \"syslog\"")
	}
	if c.InsecureSkipVerify {
		// not fatal, but the server logs a warning at startup
	}
	if err := c.validateUserAttrs(); err != nil {
		return err
	}
	return nil
}

// attrNamePattern is the LDAP attribute descriptor charset (RFC 4512 keystring).
var attrNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

// reservedUserAttrs are attributes weft manages through dedicated fields; a
// [[user_attr]] must not shadow them (lowercase for case-insensitive matching).
var reservedUserAttrs = map[string]bool{
	"uid": true, "cn": true, "sn": true, "givenname": true, "displayname": true,
	"userpassword": true, "objectclass": true,
	"uidnumber": true, "gidnumber": true, "homedirectory": true,
	"loginshell": true, "gecos": true, "memberuid": true,
}

func (c Config) validateUserAttrs() error {
	seen := map[string]bool{}
	for _, a := range c.UserAttrs {
		if !attrNamePattern.MatchString(a.Attr) {
			return fmt.Errorf("config: user_attr %q: invalid attribute name", a.Attr)
		}
		lower := strings.ToLower(a.Attr)
		if reservedUserAttrs[lower] ||
			lower == strings.ToLower(c.MailAttr) ||
			(c.MailAliasAttr != "" && lower == strings.ToLower(c.MailAliasAttr)) {
			return fmt.Errorf("config: user_attr %q collides with a built-in attribute", a.Attr)
		}
		if seen[lower] {
			return fmt.Errorf("config: user_attr %q is listed twice", a.Attr)
		}
		seen[lower] = true
		seenValues := map[string]bool{}
		for _, o := range a.Options {
			if seenValues[o.Value] {
				return fmt.Errorf("config: user_attr %q: option %q is listed twice", a.Attr, o.Value)
			}
			seenValues[o.Value] = true
		}
	}
	for _, oc := range c.UserExtraClasses {
		if !attrNamePattern.MatchString(oc) {
			return fmt.Errorf("config: user_extra_classes %q: invalid objectClass name", oc)
		}
	}
	return nil
}

// UserAttrByName returns the configured extra attribute with the given LDAP
// name (case-insensitive), or false.
func (c Config) UserAttrByName(name string) (UserAttr, bool) {
	for _, a := range c.UserAttrs {
		if strings.EqualFold(a.Attr, name) {
			return a, true
		}
	}
	return UserAttr{}, false
}
