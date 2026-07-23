// Package fake provides an in-memory directory.Directory for tests and local
// development. It mirrors the authorization model of OpenBSD ldapd under weft:
// the admin connection (rootdn) may write anything, a user connection may only
// modify its own entry. Bind credentials are stored separately from the
// userPassword attribute, exactly as in reality: weft writes a hash into
// userPassword, while the server verifies binds by its own means.
package fake

import (
	"context"
	"sort"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"weft/internal/directory"
	"weft/internal/idalloc"
)

// Fake is an in-memory directory shared by all connections it hands out.
type Fake struct {
	mu          sync.Mutex
	users       map[string]*directory.User
	groups      map[string]*directory.Group
	baseCreated bool              // base/suffix entry exists
	ous         map[string]bool   // organizationalUnit names that exist
	creds       map[string]string // uid -> plaintext bind password (unit tests)
	hashes      map[string]string // uid -> userPassword value (e.g. {CRYPT}$2b$..)
	adminPass   string
	uidRange    idalloc.Range
	gidRange    idalloc.Range
	provisioned bool
}

// New returns an empty, unprovisioned Fake with the given admin password and
// allocation ranges.
func New(adminPass string, uidRange, gidRange idalloc.Range) *Fake {
	return &Fake{
		users:     map[string]*directory.User{},
		groups:    map[string]*directory.Group{},
		ous:       map[string]bool{},
		creds:     map[string]string{},
		hashes:    map[string]string{},
		adminPass: adminPass,
		uidRange:  uidRange,
		gidRange:  gidRange,
	}
}

// SetProvisioned marks the base structure as present (or absent).
func (f *Fake) SetProvisioned(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.provisioned = v
}

// AddUser injects a user with a bind password directly (test helper).
func (f *Fake) AddUser(u directory.User, password string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[u.UID] = cloneUser(&u)
	f.creds[u.UID] = password
}

// AddGroup injects a group directly (test helper).
func (f *Fake) AddGroup(g directory.Group) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups[g.CN] = cloneGroup(&g)
}

// --- directory.Directory ---

func (f *Fake) BindUser(_ context.Context, uid, password string) (directory.Conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Unit tests inject a plaintext credential via AddUser; API/dev-created
	// users are verified against the stored {CRYPT} hash, like real ldapd.
	if want, ok := f.creds[uid]; ok && want == password {
		return &conn{f: f, boundUID: uid}, nil
	}
	if h, ok := f.hashes[uid]; ok && verifyCrypt(h, password) {
		return &conn{f: f, boundUID: uid}, nil
	}
	return nil, directory.ErrInvalidCredentials
}

// verifyCrypt checks a plaintext password against a "{CRYPT}$2b$..." value.
func verifyCrypt(stored, plain string) bool {
	raw := strings.TrimPrefix(stored, "{CRYPT}")
	return bcrypt.CompareHashAndPassword([]byte(raw), []byte(plain)) == nil
}

func (f *Fake) BindAdmin(_ context.Context, password string) (directory.Conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if password != f.adminPass {
		return nil, directory.ErrInvalidCredentials
	}
	return &conn{f: f, admin: true}, nil
}

func (f *Fake) Provisioned(_ context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.provisioned || f.ous["people"], nil
}

// conn is a bound connection over the shared Fake store.
type conn struct {
	f        *Fake
	boundUID string
	admin    bool
}

func (c *conn) WhoAmI() string {
	if c.admin {
		return "admin"
	}
	return "uid=" + c.boundUID
}
func (c *conn) IsAdmin() bool { return c.admin }
func (c *conn) Close() error  { return nil }

// ownsOrAdmin reports whether the connection may write the given uid's entry.
func (c *conn) ownsOrAdmin(uid string) bool { return c.admin || c.boundUID == uid }

func (c *conn) ListUsers(_ context.Context, term string) ([]directory.User, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	term = strings.ToLower(term)
	out := make([]directory.User, 0, len(c.f.users))
	for _, u := range c.f.users {
		if term == "" || matchesTerm(u, term) {
			out = append(out, *cloneUser(u))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out, nil
}

func (c *conn) GetUser(_ context.Context, uid string) (*directory.User, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	u, ok := c.f.users[uid]
	if !ok {
		return nil, directory.ErrNotFound
	}
	return cloneUser(u), nil
}

func (c *conn) CreateUser(_ context.Context, u directory.User, hashedPassword string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	if _, ok := c.f.users[u.UID]; ok {
		return directory.ErrAlreadyExists
	}
	c.f.users[u.UID] = cloneUser(&u)
	c.f.hashes[u.UID] = hashedPassword
	return nil
}

func (c *conn) UpdateUser(_ context.Context, u directory.User) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.ownsOrAdmin(u.UID) {
		return directory.ErrPermission
	}
	if _, ok := c.f.users[u.UID]; !ok {
		return directory.ErrNotFound
	}
	c.f.users[u.UID] = cloneUser(&u)
	return nil
}

func (c *conn) DeleteUser(_ context.Context, uid string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	if _, ok := c.f.users[uid]; !ok {
		return directory.ErrNotFound
	}
	delete(c.f.users, uid)
	delete(c.f.creds, uid)
	delete(c.f.hashes, uid)
	// Drop supplementary memberships.
	for _, g := range c.f.groups {
		g.MemberUID = removeString(g.MemberUID, uid)
	}
	return nil
}

func (c *conn) SetPassword(_ context.Context, uid, hashedPassword string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.ownsOrAdmin(uid) {
		return directory.ErrPermission
	}
	if _, ok := c.f.users[uid]; !ok {
		return directory.ErrNotFound
	}
	c.f.hashes[uid] = hashedPassword
	delete(c.f.creds, uid) // a reset supersedes any injected plaintext credential
	return nil
}

func (c *conn) RenameUID(_ context.Context, oldUID, newUID string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	u, ok := c.f.users[oldUID]
	if !ok {
		return directory.ErrNotFound
	}
	if _, exists := c.f.users[newUID]; exists {
		return directory.ErrAlreadyExists
	}
	// add-new -> fixup memberUid -> delete-old (ldapd has no ModifyDN).
	nu := cloneUser(u)
	nu.UID = newUID
	c.f.users[newUID] = nu
	if pw, ok := c.f.creds[oldUID]; ok {
		c.f.creds[newUID] = pw
	}
	if h, ok := c.f.hashes[oldUID]; ok {
		c.f.hashes[newUID] = h
	}
	for _, g := range c.f.groups {
		if g.HasMember(oldUID) {
			g.MemberUID = removeString(g.MemberUID, oldUID)
			g.MemberUID = append(g.MemberUID, newUID)
		}
	}
	delete(c.f.users, oldUID)
	delete(c.f.creds, oldUID)
	delete(c.f.hashes, oldUID)
	return nil
}

func (c *conn) CreateBaseDN(_ context.Context) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	if c.f.baseCreated {
		return directory.ErrAlreadyExists
	}
	c.f.baseCreated = true
	return nil
}

func (c *conn) CreateOU(_ context.Context, name string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	if c.f.ous[name] {
		return directory.ErrAlreadyExists
	}
	c.f.ous[name] = true
	return nil
}

func (c *conn) ListGroups(_ context.Context) ([]directory.Group, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	out := make([]directory.Group, 0, len(c.f.groups))
	for _, g := range c.f.groups {
		out = append(out, *cloneGroup(g))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CN < out[j].CN })
	return out, nil
}

func (c *conn) GetGroup(_ context.Context, cn string) (*directory.Group, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	g, ok := c.f.groups[cn]
	if !ok {
		return nil, directory.ErrNotFound
	}
	return cloneGroup(g), nil
}

func (c *conn) CreateGroup(_ context.Context, g directory.Group) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	if _, ok := c.f.groups[g.CN]; ok {
		return directory.ErrAlreadyExists
	}
	c.f.groups[g.CN] = cloneGroup(&g)
	return nil
}

func (c *conn) DeleteGroup(_ context.Context, cn string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	if _, ok := c.f.groups[cn]; !ok {
		return directory.ErrNotFound
	}
	delete(c.f.groups, cn)
	return nil
}

func (c *conn) AddMember(_ context.Context, cn, uid string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	g, ok := c.f.groups[cn]
	if !ok {
		return directory.ErrNotFound
	}
	if !g.HasMember(uid) {
		g.MemberUID = append(g.MemberUID, uid)
	}
	return nil
}

func (c *conn) RemoveMember(_ context.Context, cn, uid string) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return directory.ErrPermission
	}
	g, ok := c.f.groups[cn]
	if !ok {
		return directory.ErrNotFound
	}
	g.MemberUID = removeString(g.MemberUID, uid)
	return nil
}

func (c *conn) EffectiveGroups(_ context.Context, uid string) ([]directory.Group, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	u, ok := c.f.users[uid]
	if !ok {
		return nil, directory.ErrNotFound
	}
	seen := map[string]bool{}
	var out []directory.Group
	// Primary group via gidNumber.
	if u.POSIX != nil {
		for _, g := range c.f.groups {
			if g.GIDNumber == u.POSIX.GIDNumber {
				out = append(out, *cloneGroup(g))
				seen[g.CN] = true
			}
		}
	}
	// Supplementary via memberUid.
	for _, g := range c.f.groups {
		if !seen[g.CN] && g.HasMember(uid) {
			out = append(out, *cloneGroup(g))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CN < out[j].CN })
	return out, nil
}

func (c *conn) AllocateUIDNumber(_ context.Context) (int, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return 0, directory.ErrPermission
	}
	var used []int
	for _, u := range c.f.users {
		if u.POSIX != nil {
			used = append(used, u.POSIX.UIDNumber)
		}
	}
	return allocOrErr(c.f.uidRange, used)
}

func (c *conn) AllocateGIDNumber(_ context.Context) (int, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if !c.admin {
		return 0, directory.ErrPermission
	}
	var used []int
	for _, g := range c.f.groups {
		used = append(used, g.GIDNumber)
	}
	for _, u := range c.f.users {
		if u.POSIX != nil {
			used = append(used, u.POSIX.GIDNumber)
		}
	}
	return allocOrErr(c.f.gidRange, used)
}

func allocOrErr(r idalloc.Range, used []int) (int, error) {
	n, err := idalloc.NextFree(r, used)
	if err != nil {
		return 0, directory.ErrRangeExhausted
	}
	return n, nil
}

// --- helpers ---

func matchesTerm(u *directory.User, term string) bool {
	return strings.Contains(strings.ToLower(u.UID), term) ||
		strings.Contains(strings.ToLower(u.CN), term) ||
		strings.Contains(strings.ToLower(u.DisplayName), term)
}

func removeString(s []string, v string) []string {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func cloneUser(u *directory.User) *directory.User {
	cp := *u
	if u.POSIX != nil {
		p := *u.POSIX
		cp.POSIX = &p
	}
	if u.Mail != nil {
		m := *u.Mail
		m.Aliases = append([]string(nil), u.Mail.Aliases...)
		cp.Mail = &m
	}
	if u.Extra != nil {
		cp.Extra = make(map[string]string, len(u.Extra))
		for k, v := range u.Extra {
			cp.Extra[k] = v
		}
	}
	return &cp
}

func cloneGroup(g *directory.Group) *directory.Group {
	cp := *g
	cp.MemberUID = append([]string(nil), g.MemberUID...)
	return &cp
}
