// Package directory abstracts the external LDAP server behind an interface so
// that ldapd-specific behaviour is isolated, the application logic is testable
// against a Fake, and a different target (e.g. OpenLDAP) could be added later.
//
// Authorization is performed by the directory server, not by this package: each
// weft session holds one Conn bound as the logged-in identity (passthrough
// bind). For OpenBSD ldapd that means the rootdn (admin) may write anything,
// while a regular user may only modify their own entry ("by self" ACL). The
// Fake mirrors this so tests exercise the same authorization expectations.
package directory

import (
	"context"
	"errors"
)

// Sentinel errors. Implementations wrap these so callers can use errors.Is.
var (
	// ErrInvalidCredentials is returned by Bind when the DN/password is rejected.
	ErrInvalidCredentials = errors.New("directory: invalid credentials")
	// ErrNotFound is returned when a user or group does not exist.
	ErrNotFound = errors.New("directory: not found")
	// ErrAlreadyExists is returned when creating an entry whose RDN is taken.
	ErrAlreadyExists = errors.New("directory: already exists")
	// ErrPermission is returned when the bound identity may not perform the op.
	ErrPermission = errors.New("directory: insufficient access")
	// ErrNotProvisioned is returned by operations when the base structure
	// (ou=people, ou=groups, default group) is missing -- triggers setup mode.
	ErrNotProvisioned = errors.New("directory: base structure not provisioned")
	// ErrRangeExhausted is returned when no free uid/gid number is available.
	ErrRangeExhausted = errors.New("directory: id range exhausted")
)

// Directory is the entry point: it knows how to bind. All data operations are
// performed through the returned Conn, scoped to the bound identity.
type Directory interface {
	// BindUser binds as uid=<uid>,ou=people,<base> using the supplied password.
	// Returns ErrInvalidCredentials on failure.
	BindUser(ctx context.Context, uid, password string) (Conn, error)

	// BindAdmin binds as the configured admin DN (the ldapd rootdn).
	BindAdmin(ctx context.Context, password string) (Conn, error)

	// Provisioned reports whether the base structure exists. Used to decide
	// whether the setup wizard must run. It binds anonymously or with a short
	// read and does not require credentials beyond what the server allows.
	Provisioned(ctx context.Context) (bool, error)
}

// Conn is a connection bound as a specific identity. It is not safe for
// concurrent use; each weft session owns one Conn. Close releases it.
type Conn interface {
	// WhoAmI returns the DN this connection is bound as.
	WhoAmI() string
	// IsAdmin reports whether this connection is bound as the admin (rootdn).
	IsAdmin() bool

	// --- Users ---

	// ListUsers returns all users. The optional substring term filters on uid,
	// cn and displayName (case-insensitive); empty term returns everything.
	ListUsers(ctx context.Context, term string) ([]User, error)
	GetUser(ctx context.Context, uid string) (*User, error)
	// CreateUser adds the user with the given pre-hashed userPassword value
	// (e.g. "{CRYPT}$2b$..."). hashedPassword must not be empty.
	CreateUser(ctx context.Context, u User, hashedPassword string) error
	UpdateUser(ctx context.Context, u User) error
	DeleteUser(ctx context.Context, uid string) error

	// SetPassword sets userPassword to the given pre-hashed value (e.g.
	// "{CRYPT}$2b$...."). weft always hashes client-side; the directory never
	// hashes on write and never reads the hash back.
	SetPassword(ctx context.Context, uid, hashedPassword string) error

	// RenameUID changes a user's uid. Because ldapd does not implement ModifyDN,
	// implementations perform add-new -> fixup memberUid in all supplementary
	// groups -> delete-old. Not atomic; see README.
	RenameUID(ctx context.Context, oldUID, newUID string) error

	// CreateBaseDN creates the base/suffix entry itself (e.g. dc=example,dc=org).
	// ldapd does not auto-create the namespace suffix, so children cannot be
	// added until it exists. Used by the setup wizard before CreateOU. Returns
	// ErrAlreadyExists if the base is already present.
	CreateBaseDN(ctx context.Context) error

	// CreateOU creates an organizationalUnit "ou=<name>,<base>". Used by the
	// setup wizard. Returns ErrAlreadyExists if it is already present.
	CreateOU(ctx context.Context, name string) error

	// --- Groups ---

	ListGroups(ctx context.Context) ([]Group, error)
	GetGroup(ctx context.Context, cn string) (*Group, error)
	CreateGroup(ctx context.Context, g Group) error
	DeleteGroup(ctx context.Context, cn string) error
	AddMember(ctx context.Context, cn, uid string) error
	RemoveMember(ctx context.Context, cn, uid string) error

	// EffectiveGroups returns the user's primary group (matched via the user's
	// gidNumber) plus every posixGroup listing the uid in memberUid.
	EffectiveGroups(ctx context.Context, uid string) ([]Group, error)

	// --- ID allocation ---

	// AllocateUIDNumber / AllocateGIDNumber return the next free number within
	// the configured range. Callers must serialise allocation with an app-side
	// lock (ldapd has no atomic counters).
	AllocateUIDNumber(ctx context.Context) (int, error)
	AllocateGIDNumber(ctx context.Context) (int, error)

	// Close releases the underlying connection.
	Close() error
}
