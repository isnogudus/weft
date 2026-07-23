// Package ldapclient implements directory.Directory against a real LDAP
// server. Two server flavors are supported, selected by config
// (directory = "ldapd" | "openldap"):
//
//   - OpenBSD ldapd(8), the original target. Its verified quirks shape the
//     shared code: it has no ModifyDN (RenameUID is add-new -> fixup memberUid
//     -> delete-old), and it enforces loaded schema, so add/modify carry the
//     full objectClass chain and all MUST attributes.
//   - OpenLDAP (slapd). Standard LDAPv3 semantics; the only behavioural
//     difference in weft is RenameUID, which uses ModifyDN (atomic for the
//     entry) followed by the same memberUid fixup (memberUid values are plain
//     uid strings and do not rename with the entry).
//
// Everything else -- filters, objectClasses, {CRYPT} passwords, error mapping,
// id allocation -- is plain LDAP and identical for both. userPassword is
// written pre-hashed as "{CRYPT}$2b$..."; the server verifies it on bind, weft
// never reads it back.
//
// Authorization is delegated to the server: each Conn is bound as the
// logged-in identity (admin = rootdn, otherwise the user's own DN). The
// server's ACLs decide what the bound identity may write; this package
// surfaces a denial as directory.ErrPermission.
package ldapclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"weft/internal/config"
	"weft/internal/directory"
	"weft/internal/idalloc"
)

// objectClass chains for the entries weft manages.
var (
	userClasses  = []string{"top", "person", "organizationalPerson", "inetOrgPerson"}
	posixClass   = "posixAccount"
	groupClasses = []string{"top", "posixGroup"}
)

// Directory dials and binds against the configured LDAP server.
type Directory struct {
	cfg     config.Config
	tlsCfg  *tls.Config              // built once at New(); nil for ldapi/plain
	dialRaw func() (net.Conn, error) // opens a raw (un-TLS'd) connection
}

// New returns a Directory for the given configuration. dialRaw opens a raw,
// un-TLS'd connection to the LDAP endpoint; pass nil to use the default network
// dialer (DNS + connect). Under privilege separation the worker passes a dialer
// that requests a connected fd from the monitor instead.
//
// It builds the TLS configuration up front -- reading the CA file or capturing
// the system trust store now -- so that no certificate file is read after the
// process is sandboxed (chroot/unveil) at startup.
func New(cfg config.Config, dialRaw func() (net.Conn, error)) (*Directory, error) {
	d := &Directory{cfg: cfg, dialRaw: dialRaw}
	if !cfg.IsLDAPI() && cfg.TLSMode != config.TLSPlain {
		t, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		d.tlsCfg = t
	}
	if d.dialRaw == nil {
		network, address, err := cfg.DialTarget()
		if err != nil {
			return nil, err
		}
		d.dialRaw = func() (net.Conn, error) {
			return net.DialTimeout(network, address, 10*time.Second)
		}
	}
	return d, nil
}

// --- dialing / TLS ---

func buildTLSConfig(cfg config.Config) (*tls.Config, error) {
	u, err := url.Parse(cfg.LDAPURL)
	if err != nil {
		return nil, fmt.Errorf("ldap: parse url: %w", err)
	}
	t := &tls.Config{
		ServerName:         u.Hostname(),
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // opt-in via config/-insecure; warned at startup
		MinVersion:         tls.VersionTLS12,
	}
	if cfg.InsecureSkipVerify {
		return t, nil
	}
	if cfg.CACertFile != "" {
		pem, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("ldap: read ca cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ldap: no certs found in %s", cfg.CACertFile)
		}
		t.RootCAs = pool
		return t, nil
	}
	// Capture the system trust store now, before any sandbox locks the FS.
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("ldap: load system cert pool: %w", err)
	}
	t.RootCAs = pool
	return t, nil
}

// dial opens a raw connection (network or via the privsep monitor) and wraps it
// into an LDAP connection, applying TLS according to tls_mode.
func (d *Directory) dial() (*ldap.Conn, error) {
	raw, err := d.dialRaw()
	if err != nil {
		return nil, err
	}
	return d.wrap(raw)
}

// wrap turns a raw transport connection into a started *ldap.Conn. The
// connection's TLS handshake (if any) uses the pre-built tlsCfg, so no
// certificate file is read here -- important once the process is sandboxed.
func (d *Directory) wrap(raw net.Conn) (*ldap.Conn, error) {
	// ldapi (local socket) and explicit plain: no TLS.
	if d.cfg.IsLDAPI() || d.cfg.TLSMode == config.TLSPlain {
		l := ldap.NewConn(raw, false)
		l.Start()
		return l, nil
	}
	switch d.cfg.TLSMode {
	case config.TLSLDAPS:
		tconn := tls.Client(raw, d.tlsCfg)
		if err := tconn.Handshake(); err != nil {
			raw.Close()
			return nil, fmt.Errorf("ldap: tls handshake: %w", err)
		}
		l := ldap.NewConn(tconn, true)
		l.Start()
		return l, nil
	case config.TLSStartTLS:
		l := ldap.NewConn(raw, false)
		l.Start()
		if err := l.StartTLS(d.tlsCfg); err != nil {
			l.Close()
			return nil, fmt.Errorf("ldap: starttls: %w", err)
		}
		return l, nil
	default:
		raw.Close()
		return nil, fmt.Errorf("ldap: unknown tls mode %q", d.cfg.TLSMode)
	}
}

func (d *Directory) bind(dn, password string, admin bool) (directory.Conn, error) {
	c, err := d.dial()
	if err != nil {
		return nil, fmt.Errorf("ldap: dial: %w", err)
	}
	if err := c.Bind(dn, password); err != nil {
		c.Close()
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return nil, directory.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("ldap: bind: %w", err)
	}
	return &conn{d: d, lc: c, dn: dn, admin: admin}, nil
}

// BindUser binds as uid=<uid>,ou=people,<base>.
func (d *Directory) BindUser(_ context.Context, uid, password string) (directory.Conn, error) {
	return d.bind(d.cfg.UserDN(uid), password, false)
}

// BindAdmin binds as the configured admin DN (the server's rootdn).
func (d *Directory) BindAdmin(_ context.Context, password string) (directory.Conn, error) {
	return d.bind(d.cfg.AdminBindDN(), password, true)
}

// Provisioned checks (anonymously) whether ou=people exists under the base.
func (d *Directory) Provisioned(_ context.Context) (bool, error) {
	c, err := d.dial()
	if err != nil {
		return false, fmt.Errorf("ldap: dial: %w", err)
	}
	defer c.Close()
	req := ldap.NewSearchRequest(
		d.cfg.PeopleDN(), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=organizationalUnit)", []string{"ou"}, nil)
	res, err := c.Search(req)
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return false, nil
		}
		return false, fmt.Errorf("ldap: provisioned check: %w", err)
	}
	return len(res.Entries) > 0, nil
}

// --- conn ---

type conn struct {
	d     *Directory
	lc    *ldap.Conn
	dn    string
	admin bool
}

func (c *conn) WhoAmI() string { return c.dn }
func (c *conn) IsAdmin() bool  { return c.admin }
func (c *conn) Close() error   { return c.lc.Close() }

// mapErr translates ldap result codes into directory sentinel errors.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject):
		return directory.ErrNotFound
	case ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists):
		return directory.ErrAlreadyExists
	case ldap.IsErrorWithCode(err, ldap.LDAPResultInsufficientAccessRights):
		return directory.ErrPermission
	default:
		return err
	}
}

// --- users ---

var userAttrs = []string{
	"objectClass", "uid", "cn", "sn", "givenName", "displayName",
	"uidNumber", "gidNumber", "homeDirectory", "loginShell", "gecos",
}

func (c *conn) userSearchAttrs() []string {
	a := append([]string(nil), userAttrs...)
	a = append(a, c.d.cfg.MailAttr)
	if c.d.cfg.MailAliasAttr != "" {
		a = append(a, c.d.cfg.MailAliasAttr)
	}
	for _, ua := range c.d.cfg.UserAttrs {
		a = append(a, ua.Attr)
	}
	return a
}

func (c *conn) ListUsers(_ context.Context, term string) ([]directory.User, error) {
	filter := "(objectClass=inetOrgPerson)"
	if term != "" {
		t := ldap.EscapeFilter(term)
		filter = fmt.Sprintf("(&(objectClass=inetOrgPerson)(|(%s=*%s*)(cn=*%s*)(displayName=*%s*)))",
			c.d.cfg.UserIDAttr, t, t, t)
	}
	req := ldap.NewSearchRequest(
		c.d.cfg.PeopleDN(), ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		filter, c.userSearchAttrs(), nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]directory.User, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, *c.parseUser(e))
	}
	return out, nil
}

func (c *conn) GetUser(_ context.Context, uid string) (*directory.User, error) {
	req := ldap.NewSearchRequest(
		c.d.cfg.UserDN(uid), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=inetOrgPerson)", c.userSearchAttrs(), nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	if len(res.Entries) == 0 {
		return nil, directory.ErrNotFound
	}
	return c.parseUser(res.Entries[0]), nil
}

func (c *conn) parseUser(e *ldap.Entry) *directory.User {
	// When UserIDAttr is "cn", this and the CN field below read the SAME LDAP
	// attribute -- they naturally end up equal, which is correct: the RDN
	// value must be one of the entry's own cn values (see UpdateUser/CreateUser).
	u := &directory.User{
		UID:         e.GetAttributeValue(c.d.cfg.UserIDAttr),
		CN:          e.GetAttributeValue("cn"),
		SN:          e.GetAttributeValue("sn"),
		GivenName:   e.GetAttributeValue("givenName"),
		DisplayName: e.GetAttributeValue("displayName"),
	}
	if hasValue(e.GetAttributeValues("objectClass"), posixClass) {
		u.POSIX = &directory.POSIXProfile{
			UIDNumber:     atoi(e.GetAttributeValue("uidNumber")),
			GIDNumber:     atoi(e.GetAttributeValue("gidNumber")),
			HomeDirectory: e.GetAttributeValue("homeDirectory"),
			LoginShell:    e.GetAttributeValue("loginShell"),
			Gecos:         e.GetAttributeValue("gecos"),
		}
	}
	if m := c.parseMail(e); m != nil {
		u.Mail = m
	}
	if len(c.d.cfg.UserAttrs) > 0 {
		extra := make(map[string]string)
		for _, ua := range c.d.cfg.UserAttrs {
			if v := e.GetAttributeValue(ua.Attr); v != "" {
				extra[ua.Attr] = v
			}
		}
		if len(extra) > 0 {
			u.Extra = extra
		}
	}
	return u
}

func (c *conn) parseMail(e *ldap.Entry) *directory.MailProfile {
	primary := e.GetAttributeValues(c.d.cfg.MailAttr)
	if c.d.cfg.MailAliasAttr == "" {
		if len(primary) == 0 {
			return nil
		}
		return &directory.MailProfile{Mail: primary[0], Aliases: primary[1:]}
	}
	aliases := e.GetAttributeValues(c.d.cfg.MailAliasAttr)
	if len(primary) == 0 && len(aliases) == 0 {
		return nil
	}
	m := &directory.MailProfile{Aliases: aliases}
	if len(primary) > 0 {
		m.Mail = primary[0]
	}
	return m
}

func (c *conn) CreateUser(_ context.Context, u directory.User, hashedPassword string) error {
	req := ldap.NewAddRequest(c.d.cfg.UserDN(u.UID), nil)
	classes := append([]string(nil), userClasses...)
	if u.POSIX != nil {
		classes = append(classes, posixClass)
	}
	classes = append(classes, c.d.cfg.UserExtraClasses...)
	req.Attribute("objectClass", classes)
	// When UserIDAttr is "cn" there is no separate uid attribute at all: the
	// RDN (built from u.UID via UserDN) must be one of cn's own values, and
	// the service layer already sets u.CN == u.UID in that mode, so the cn
	// write below covers it.
	if c.d.cfg.UserIDAttr != "cn" {
		req.Attribute("uid", []string{u.UID})
	}
	req.Attribute("cn", []string{u.CN})
	req.Attribute("sn", []string{u.SN})
	addIf(req, "givenName", u.GivenName)
	addIf(req, "displayName", u.DisplayName)
	req.Attribute("userPassword", []string{hashedPassword})
	if u.POSIX != nil {
		req.Attribute("uidNumber", []string{itoa(u.POSIX.UIDNumber)})
		req.Attribute("gidNumber", []string{itoa(u.POSIX.GIDNumber)})
		req.Attribute("homeDirectory", []string{u.POSIX.HomeDirectory})
		addIf(req, "loginShell", u.POSIX.LoginShell)
		addIf(req, "gecos", u.POSIX.Gecos)
	}
	c.addMail(req, u.Mail)
	for _, ua := range c.d.cfg.UserAttrs {
		addIf(req, ua.Attr, u.Extra[ua.Attr])
	}
	return mapErr(c.lc.Add(req))
}

// addMail appends mail attributes to an AddRequest.
func (c *conn) addMail(req *ldap.AddRequest, m *directory.MailProfile) {
	if m == nil {
		return
	}
	if c.d.cfg.MailAliasAttr == "" {
		vals := append([]string{}, m.Mail)
		vals = append(vals, m.Aliases...)
		vals = nonEmpty(vals)
		if len(vals) > 0 {
			req.Attribute(c.d.cfg.MailAttr, vals)
		}
		return
	}
	if m.Mail != "" {
		req.Attribute(c.d.cfg.MailAttr, []string{m.Mail})
	}
	if len(m.Aliases) > 0 {
		req.Attribute(c.d.cfg.MailAliasAttr, m.Aliases)
	}
}

func (c *conn) UpdateUser(ctx context.Context, u directory.User) error {
	cur, err := c.GetUser(ctx, u.UID)
	if err != nil {
		return err
	}
	m := ldap.NewModifyRequest(c.d.cfg.UserDN(u.UID), nil)
	m.Replace("cn", []string{u.CN})
	m.Replace("sn", []string{u.SN})
	m.Replace("givenName", nonEmpty([]string{u.GivenName}))
	m.Replace("displayName", nonEmpty([]string{u.DisplayName}))

	// POSIX profile toggle.
	switch {
	case u.POSIX != nil && cur.POSIX != nil:
		m.Replace("uidNumber", []string{itoa(u.POSIX.UIDNumber)})
		m.Replace("gidNumber", []string{itoa(u.POSIX.GIDNumber)})
		m.Replace("homeDirectory", []string{u.POSIX.HomeDirectory})
		m.Replace("loginShell", nonEmpty([]string{u.POSIX.LoginShell}))
		m.Replace("gecos", nonEmpty([]string{u.POSIX.Gecos}))
	case u.POSIX != nil && cur.POSIX == nil:
		m.Add("objectClass", []string{posixClass})
		m.Add("uidNumber", []string{itoa(u.POSIX.UIDNumber)})
		m.Add("gidNumber", []string{itoa(u.POSIX.GIDNumber)})
		m.Add("homeDirectory", []string{u.POSIX.HomeDirectory})
		addIfMod(m, "loginShell", u.POSIX.LoginShell)
		addIfMod(m, "gecos", u.POSIX.Gecos)
	case u.POSIX == nil && cur.POSIX != nil:
		m.Replace("uidNumber", nil)
		m.Replace("gidNumber", nil)
		m.Replace("homeDirectory", nil)
		m.Replace("loginShell", nil)
		m.Replace("gecos", nil)
		m.Delete("objectClass", []string{posixClass})
	}

	// Mail profile (no objectClass needed for the default "mail" attribute).
	c.modifyMail(m, u.Mail)

	// Extra attributes. Entries created before user_extra_classes was
	// configured may lack the auxiliary classes; add the missing ones so the
	// schema accepts the attributes.
	if len(c.d.cfg.UserAttrs) > 0 {
		if len(c.d.cfg.UserExtraClasses) > 0 {
			missing, err := c.missingExtraClasses(u.UID)
			if err != nil {
				return err
			}
			if len(missing) > 0 {
				m.Add("objectClass", missing)
			}
		}
		for _, ua := range c.d.cfg.UserAttrs {
			m.Replace(ua.Attr, nonEmpty([]string{u.Extra[ua.Attr]}))
		}
	}

	return mapErr(c.lc.Modify(m))
}

// missingExtraClasses returns the configured auxiliary classes the entry does
// not carry yet.
func (c *conn) missingExtraClasses(uid string) ([]string, error) {
	req := ldap.NewSearchRequest(
		c.d.cfg.UserDN(uid), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"objectClass"}, nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	if len(res.Entries) == 0 {
		return nil, directory.ErrNotFound
	}
	have := res.Entries[0].GetAttributeValues("objectClass")
	var missing []string
	for _, oc := range c.d.cfg.UserExtraClasses {
		if !hasValue(have, oc) {
			missing = append(missing, oc)
		}
	}
	return missing, nil
}

func (c *conn) modifyMail(m *ldap.ModifyRequest, mail *directory.MailProfile) {
	if c.d.cfg.MailAliasAttr == "" {
		var vals []string
		if mail != nil {
			vals = nonEmpty(append([]string{mail.Mail}, mail.Aliases...))
		}
		m.Replace(c.d.cfg.MailAttr, vals)
		return
	}
	var primary, aliases []string
	if mail != nil {
		primary = nonEmpty([]string{mail.Mail})
		aliases = mail.Aliases
	}
	m.Replace(c.d.cfg.MailAttr, primary)
	m.Replace(c.d.cfg.MailAliasAttr, aliases)
}

func (c *conn) SetPassword(_ context.Context, uid, hashedPassword string) error {
	m := ldap.NewModifyRequest(c.d.cfg.UserDN(uid), nil)
	m.Replace("userPassword", []string{hashedPassword})
	return mapErr(c.lc.Modify(m))
}

func (c *conn) DeleteUser(ctx context.Context, uid string) error {
	if err := mapErr(c.lc.Del(ldap.NewDelRequest(c.d.cfg.UserDN(uid), nil))); err != nil {
		return err
	}
	return c.removeFromAllGroups(ctx, uid)
}

func (c *conn) RenameUID(ctx context.Context, oldUID, newUID string) error {
	if c.d.cfg.Directory == config.DirectoryOpenLDAP {
		return c.renameModifyDN(ctx, oldUID, newUID)
	}
	return c.renameCopyDelete(ctx, oldUID, newUID)
}

// renameModifyDN renames via ModifyDN (OpenLDAP): atomic for the entry itself.
// memberUid values are plain uid strings, so the group fixup still follows.
func (c *conn) renameModifyDN(ctx context.Context, oldUID, newUID string) error {
	req := ldap.NewModifyDNRequest(c.d.cfg.UserDN(oldUID), c.d.cfg.UserRDN(newUID), true, "")
	if err := mapErr(c.lc.ModifyDN(req)); err != nil {
		return err
	}
	groups, err := c.groupsWithMember(oldUID)
	if err != nil {
		return err
	}
	for _, g := range groups {
		_ = c.AddMember(ctx, g, newUID)
		_ = c.RemoveMember(ctx, g, oldUID)
	}
	return nil
}

func (c *conn) renameCopyDelete(ctx context.Context, oldUID, newUID string) error {
	// ldapd has no ModifyDN -> add-new, fixup memberUid, delete-old.
	src := ldap.NewSearchRequest(
		c.d.cfg.UserDN(oldUID), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=inetOrgPerson)", append(c.userSearchAttrs(), "userPassword"), nil)
	res, err := c.lc.Search(src)
	if err != nil {
		return mapErr(err)
	}
	if len(res.Entries) == 0 {
		return directory.ErrNotFound
	}
	e := res.Entries[0]

	add := ldap.NewAddRequest(c.d.cfg.UserDN(newUID), nil)
	for _, a := range e.Attributes {
		vals := a.Values
		if strings.EqualFold(a.Name, c.d.cfg.UserIDAttr) {
			vals = []string{newUID}
		}
		add.Attribute(a.Name, vals)
	}
	if err := mapErr(c.lc.Add(add)); err != nil {
		return err
	}

	// Migrate supplementary memberships, then drop the old entry.
	groups, err := c.groupsWithMember(oldUID)
	if err != nil {
		return err
	}
	for _, g := range groups {
		_ = c.AddMember(ctx, g, newUID)
		_ = c.RemoveMember(ctx, g, oldUID)
	}
	return mapErr(c.lc.Del(ldap.NewDelRequest(c.d.cfg.UserDN(oldUID), nil)))
}

func (c *conn) CreateBaseDN(_ context.Context) error {
	dn := c.d.cfg.BaseDN
	parsed, err := ldap.ParseDN(dn)
	if err != nil || len(parsed.RDNs) == 0 || len(parsed.RDNs[0].Attributes) == 0 {
		return fmt.Errorf("ldap: cannot parse base_dn %q", dn)
	}
	rdn := parsed.RDNs[0].Attributes[0]
	attr, val := strings.ToLower(rdn.Type), rdn.Value

	req := ldap.NewAddRequest(dn, nil)
	switch attr {
	case "dc":
		req.Attribute("objectClass", []string{"top", "dcObject", "organization"})
		req.Attribute("dc", []string{val})
		req.Attribute("o", []string{val})
	case "o":
		req.Attribute("objectClass", []string{"top", "organization"})
		req.Attribute("o", []string{val})
	case "ou":
		req.Attribute("objectClass", []string{"top", "organizationalUnit"})
		req.Attribute("ou", []string{val})
	default:
		return fmt.Errorf("ldap: cannot auto-create base entry %q (unsupported RDN type %q) -- create it manually", dn, attr)
	}
	return mapErr(c.lc.Add(req))
}

func (c *conn) CreateOU(_ context.Context, name string) error {
	dn := "ou=" + name + "," + c.d.cfg.BaseDN
	req := ldap.NewAddRequest(dn, nil)
	req.Attribute("objectClass", []string{"top", "organizationalUnit"})
	req.Attribute("ou", []string{name})
	return mapErr(c.lc.Add(req))
}

// --- groups ---

func (c *conn) ListGroups(_ context.Context) ([]directory.Group, error) {
	req := ldap.NewSearchRequest(
		c.d.cfg.GroupsDN(), ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=posixGroup)", []string{"cn", "gidNumber", "memberUid"}, nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]directory.Group, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, *parseGroup(e))
	}
	return out, nil
}

func (c *conn) GetGroup(_ context.Context, cn string) (*directory.Group, error) {
	req := ldap.NewSearchRequest(
		c.d.cfg.GroupDN(cn), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=posixGroup)", []string{"cn", "gidNumber", "memberUid"}, nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	if len(res.Entries) == 0 {
		return nil, directory.ErrNotFound
	}
	return parseGroup(res.Entries[0]), nil
}

func parseGroup(e *ldap.Entry) *directory.Group {
	return &directory.Group{
		CN:        e.GetAttributeValue("cn"),
		GIDNumber: atoi(e.GetAttributeValue("gidNumber")),
		MemberUID: e.GetAttributeValues("memberUid"),
	}
}

func (c *conn) CreateGroup(_ context.Context, g directory.Group) error {
	req := ldap.NewAddRequest(c.d.cfg.GroupDN(g.CN), nil)
	req.Attribute("objectClass", groupClasses)
	req.Attribute("cn", []string{g.CN})
	req.Attribute("gidNumber", []string{itoa(g.GIDNumber)})
	if len(g.MemberUID) > 0 {
		req.Attribute("memberUid", g.MemberUID)
	}
	return mapErr(c.lc.Add(req))
}

func (c *conn) DeleteGroup(_ context.Context, cn string) error {
	return mapErr(c.lc.Del(ldap.NewDelRequest(c.d.cfg.GroupDN(cn), nil)))
}

func (c *conn) AddMember(_ context.Context, cn, uid string) error {
	m := ldap.NewModifyRequest(c.d.cfg.GroupDN(cn), nil)
	m.Add("memberUid", []string{uid})
	err := c.lc.Modify(m)
	if ldap.IsErrorWithCode(err, ldap.LDAPResultAttributeOrValueExists) {
		return nil // already a member
	}
	return mapErr(err)
}

func (c *conn) RemoveMember(_ context.Context, cn, uid string) error {
	m := ldap.NewModifyRequest(c.d.cfg.GroupDN(cn), nil)
	m.Delete("memberUid", []string{uid})
	err := c.lc.Modify(m)
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchAttribute) {
		return nil // not a member
	}
	return mapErr(err)
}

func (c *conn) EffectiveGroups(ctx context.Context, uid string) ([]directory.Group, error) {
	u, err := c.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	filter := fmt.Sprintf("(&(objectClass=posixGroup)(memberUid=%s))", ldap.EscapeFilter(uid))
	if u.POSIX != nil {
		filter = fmt.Sprintf("(&(objectClass=posixGroup)(|(memberUid=%s)(gidNumber=%d)))",
			ldap.EscapeFilter(uid), u.POSIX.GIDNumber)
	}
	req := ldap.NewSearchRequest(
		c.d.cfg.GroupsDN(), ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		filter, []string{"cn", "gidNumber", "memberUid"}, nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]directory.Group, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, *parseGroup(e))
	}
	return out, nil
}

// groupsWithMember returns the cns of groups listing uid in memberUid.
func (c *conn) groupsWithMember(uid string) ([]string, error) {
	req := ldap.NewSearchRequest(
		c.d.cfg.GroupsDN(), ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=posixGroup)(memberUid=%s))", ldap.EscapeFilter(uid)),
		[]string{"cn"}, nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	var cns []string
	for _, e := range res.Entries {
		cns = append(cns, e.GetAttributeValue("cn"))
	}
	return cns, nil
}

func (c *conn) removeFromAllGroups(ctx context.Context, uid string) error {
	cns, err := c.groupsWithMember(uid)
	if err != nil {
		return err
	}
	for _, cn := range cns {
		if err := c.RemoveMember(ctx, cn, uid); err != nil {
			return err
		}
	}
	return nil
}

// --- id allocation ---

func (c *conn) AllocateUIDNumber(_ context.Context) (int, error) {
	used, err := c.collectNumbers(c.d.cfg.PeopleDN(), "(objectClass=posixAccount)", "uidNumber")
	if err != nil {
		return 0, err
	}
	return allocOrErr(c.d.cfg.UIDRange(), used)
}

func (c *conn) AllocateGIDNumber(_ context.Context) (int, error) {
	used, err := c.collectNumbers(c.d.cfg.GroupsDN(), "(objectClass=posixGroup)", "gidNumber")
	if err != nil {
		return 0, err
	}
	// Also avoid clashing with user primary gids.
	userGids, err := c.collectNumbers(c.d.cfg.PeopleDN(), "(objectClass=posixAccount)", "gidNumber")
	if err != nil {
		return 0, err
	}
	return allocOrErr(c.d.cfg.GIDRange(), append(used, userGids...))
}

func (c *conn) collectNumbers(base, filter, attr string) ([]int, error) {
	req := ldap.NewSearchRequest(
		base, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		filter, []string{attr}, nil)
	res, err := c.lc.Search(req)
	if err != nil {
		return nil, mapErr(err)
	}
	var nums []int
	for _, e := range res.Entries {
		if v := e.GetAttributeValue(attr); v != "" {
			nums = append(nums, atoi(v))
		}
	}
	return nums, nil
}

func allocOrErr(r idalloc.Range, used []int) (int, error) {
	n, err := idalloc.NextFree(r, used)
	if err != nil {
		return 0, directory.ErrRangeExhausted
	}
	return n, nil
}

// --- small helpers ---

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
func itoa(n int) string { return strconv.Itoa(n) }

func hasValue(vals []string, want string) bool {
	for _, v := range vals {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func addIf(req *ldap.AddRequest, attr, val string) {
	if val != "" {
		req.Attribute(attr, []string{val})
	}
}

func addIfMod(m *ldap.ModifyRequest, attr, val string) {
	if val != "" {
		m.Add(attr, []string{val})
	}
}
