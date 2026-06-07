// Package config loads weft's configuration from (in increasing precedence)
// built-in defaults, a TOML file, environment variables (WEFT_*) and command
// line flags. The core is deliberately minimal -- ldap_url + base_dn -- with
// good defaults for everything else.
package config

import (
	"fmt"
	"net/url"
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

// Config is the fully-resolved configuration.
type Config struct {
	// Core
	LDAPURL string `toml:"ldap_url"`
	BaseDN  string `toml:"base_dn"`

	// Transport security to the LDAP server.
	TLSMode            TLSMode `toml:"tls_mode"`
	CACertFile         string  `toml:"ca_cert_file"`
	InsecureSkipVerify bool    `toml:"insecure_skip_verify"` // skip LDAP cert verification (self-signed); logs a warning
	AllowPlainBind     bool    `toml:"allow_plain_bind"`     // dev only

	// Admin identity. The admin logs in by typing AdminUID as the username; the
	// session then binds as the admin DN, which must equal ldapd's rootdn.
	//
	// AdminDN is that bind DN. Leave it empty to derive the default
	// uid=<AdminUID>,ou=people,<base>; set it explicitly when your ldapd rootdn
	// has a different shape (e.g. "cn=admin,dc=example,dc=org"). The admin is
	// synthetic -- it need not exist as a directory entry (ldapd special-cases
	// the rootpw). Resolve via AdminBindDN().
	AdminUID string `toml:"admin_uid"`
	AdminDN  string `toml:"admin_dn"`

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

	// Password hashing.
	BcryptCost        int `toml:"bcrypt_cost"`
	MaxPasswordLength int `toml:"max_password_length"` // bcrypt truncates at 72 bytes

	// HTTP server.
	ListenAddr     string   `toml:"listen_addr"`
	TLSCertFile    string   `toml:"tls_cert_file"` // optional standalone TLS
	TLSKeyFile     string   `toml:"tls_key_file"`
	SessionTimeout Duration `toml:"session_timeout"` // e.g. "30m"
	CookieSecure   bool     `toml:"cookie_secure"`   // set false only for local http dev
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
		TLSMode:           TLSLDAPS,
		AdminUID:          "admin",
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

// UserDN builds the bind/entry DN for a uid from the fixed template.
func (c Config) UserDN(uid string) string {
	return "uid=" + uid + ",ou=" + c.PeopleOU + "," + c.BaseDN
}

// GroupDN builds the entry DN for a group cn.
func (c Config) GroupDN(cn string) string {
	return "cn=" + cn + ",ou=" + c.GroupsOU + "," + c.BaseDN
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

// Validate checks the resolved configuration for consistency.
func (c Config) Validate() error {
	if c.LDAPURL == "" {
		return fmt.Errorf("config: ldap_url is required")
	}
	u, err := url.Parse(c.LDAPURL)
	if err != nil {
		return fmt.Errorf("config: invalid ldap_url: %w", err)
	}
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
	if c.BaseDN == "" {
		return fmt.Errorf("config: base_dn is required")
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
	if c.InsecureSkipVerify {
		// not fatal, but the server logs a warning at startup
	}
	return nil
}
