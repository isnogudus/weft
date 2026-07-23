package directory

// Domain model for weft. These types are deliberately decoupled from any LDAP
// schema detail; the directory implementation maps them to ldapd objectClasses
// and attributes. See README for the concrete attribute mapping.

// User is a directory user. The base profile is inetOrgPerson; the optional
// POSIX and Mail profiles can be switched on per user.
type User struct {
	UID         string // RDN, maps to attribute "uid"
	CN          string // common name (MUST on person)
	SN          string // surname (MUST on person)
	GivenName   string
	DisplayName string

	// POSIX is set when the user has a posixAccount profile (shell account).
	POSIX *POSIXProfile
	// Mail is set when the user has a mail profile.
	Mail *MailProfile

	// Extra holds the values of the configured extra attributes
	// (config.UserAttrs), keyed by LDAP attribute name. Only single-valued
	// string attributes are supported; absent/empty values are omitted.
	Extra map[string]string
}

// HasPOSIX reports whether the user carries a POSIX profile.
func (u *User) HasPOSIX() bool { return u.POSIX != nil }

// HasMail reports whether the user carries a mail profile.
func (u *User) HasMail() bool { return u.Mail != nil }

// POSIXProfile holds the posixAccount attributes. uidNumber/gidNumber are
// allocated automatically (see internal/idalloc) but may be overridden by an
// admin.
type POSIXProfile struct {
	UIDNumber     int
	GIDNumber     int
	HomeDirectory string
	LoginShell    string
	Gecos         string
}

// MailProfile is deliberately slim: a primary address plus optional aliases.
// The concrete attribute set is configurable (see config.Mail).
type MailProfile struct {
	Mail    string
	Aliases []string
}

// Group is a posixGroup. Membership is uid-based via memberUid; it never holds
// DNs. A group may be empty.
type Group struct {
	CN        string // RDN, maps to attribute "cn"
	GIDNumber int
	MemberUID []string
}

// HasMember reports whether uid is a supplementary member of the group.
func (g *Group) HasMember(uid string) bool {
	for _, m := range g.MemberUID {
		if m == uid {
			return true
		}
	}
	return false
}
