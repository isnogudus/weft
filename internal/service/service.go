// Package service holds the application logic that sits between the HTTP layer
// and the directory: input validation, password hashing, POSIX defaults and
// collision-safe uid/gid allocation. It is directory-agnostic and tested
// against the Fake.
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"weft/internal/config"
	"weft/internal/directory"
	"weft/internal/password"
)

// Service orchestrates writes. A single instance is shared across requests; its
// allocMu serialises uid/gid allocation since ldapd has no atomic counters.
type Service struct {
	cfg     config.Config
	allocMu sync.Mutex
}

// New returns a Service for the given configuration.
func New(cfg config.Config) *Service { return &Service{cfg: cfg} }

// Cfg exposes the configuration to the HTTP layer.
func (s *Service) Cfg() config.Config { return s.cfg }

// POSIXInput carries the optional POSIX profile for a create/update. Zero
// UIDNumber/GIDNumber mean "allocate" / "use primary group".
type POSIXInput struct {
	UIDNumber     int
	GIDNumber     int
	HomeDirectory string
	LoginShell    string
	Gecos         string
	PrimaryGroup  string // "" -> configured default
}

// NewUser is the input for CreateUser.
type NewUser struct {
	UID         string
	CN          string
	SN          string
	GivenName   string
	DisplayName string
	Password    string
	POSIX       *POSIXInput
	Mail        *directory.MailProfile
}

// CreateUser validates input, allocates POSIX ids if needed, hashes the
// password and writes the entry. Allocation + write run under allocMu so two
// concurrent creates cannot pick the same uidNumber.
func (s *Service) CreateUser(ctx context.Context, c directory.Conn, in NewUser) (*directory.User, error) {
	if err := validName("uid", in.UID); err != nil {
		return nil, err
	}
	if err := validText("cn", in.CN); err != nil {
		return nil, err
	}
	if err := validText("sn", in.SN); err != nil {
		return nil, err
	}
	hash, err := password.Hash(in.Password, s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}

	u := directory.User{
		UID:         in.UID,
		CN:          in.CN,
		SN:          in.SN,
		GivenName:   in.GivenName,
		DisplayName: in.DisplayName,
		Mail:        normalizeMail(in.Mail),
	}

	if in.POSIX != nil {
		s.allocMu.Lock()
		defer s.allocMu.Unlock()
		p, err := s.resolvePOSIX(ctx, c, in.UID, *in.POSIX, 0)
		if err != nil {
			return nil, err
		}
		u.POSIX = p
	}

	if err := c.CreateUser(ctx, u, hash); err != nil {
		return nil, err
	}
	return &u, nil
}

// resolvePOSIX fills in allocated/defaulted POSIX fields. currentUID is the uid
// whose existing uidNumber (if any) should be preserved when not overridden;
// pass 0/"" semantics via override fields. The caller must hold allocMu.
func (s *Service) resolvePOSIX(ctx context.Context, c directory.Conn, uid string, in POSIXInput, existingUIDNumber int) (*directory.POSIXProfile, error) {
	p := &directory.POSIXProfile{
		HomeDirectory: in.HomeDirectory,
		LoginShell:    in.LoginShell,
		Gecos:         in.Gecos,
	}
	if p.HomeDirectory == "" {
		p.HomeDirectory = s.cfg.HomeDir(uid)
	}
	if p.LoginShell == "" {
		p.LoginShell = s.cfg.DefaultShell
	}

	// uidNumber: explicit override > existing > freshly allocated.
	switch {
	case in.UIDNumber > 0:
		p.UIDNumber = in.UIDNumber
	case existingUIDNumber > 0:
		p.UIDNumber = existingUIDNumber
	default:
		n, err := c.AllocateUIDNumber(ctx)
		if err != nil {
			return nil, err
		}
		p.UIDNumber = n
	}

	// gidNumber: explicit override > primary group's gid.
	if in.GIDNumber > 0 {
		p.GIDNumber = in.GIDNumber
		return p, nil
	}
	primary := in.PrimaryGroup
	if primary == "" {
		primary = s.cfg.PrimaryGroup
	}
	g, err := c.GetGroup(ctx, primary)
	if err != nil {
		if errors.Is(err, directory.ErrNotFound) {
			return nil, fmt.Errorf("primary group %q does not exist", primary)
		}
		return nil, err
	}
	p.GIDNumber = g.GIDNumber
	return p, nil
}

// UpdateUser writes profile changes. Toggling the POSIX profile on allocates a
// uidNumber; existing POSIX numbers are preserved unless overridden.
func (s *Service) UpdateUser(ctx context.Context, c directory.Conn, in NewUser) (*directory.User, error) {
	if err := validText("cn", in.CN); err != nil {
		return nil, err
	}
	if err := validText("sn", in.SN); err != nil {
		return nil, err
	}
	cur, err := c.GetUser(ctx, in.UID)
	if err != nil {
		return nil, err
	}
	u := directory.User{
		UID:         in.UID,
		CN:          in.CN,
		SN:          in.SN,
		GivenName:   in.GivenName,
		DisplayName: in.DisplayName,
		Mail:        normalizeMail(in.Mail),
	}
	if in.POSIX != nil {
		s.allocMu.Lock()
		defer s.allocMu.Unlock()
		existing := 0
		if cur.POSIX != nil {
			existing = cur.POSIX.UIDNumber
		}
		p, err := s.resolvePOSIX(ctx, c, in.UID, *in.POSIX, existing)
		if err != nil {
			return nil, err
		}
		u.POSIX = p
	}
	if err := c.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	return &u, nil
}

// SetPassword hashes a new password and writes it for uid. Authorization is the
// directory's job (admin may set anyone's; a user only their own).
func (s *Service) SetPassword(ctx context.Context, c directory.Conn, uid, newPassword string) error {
	hash, err := password.Hash(newPassword, s.cfg.BcryptCost)
	if err != nil {
		return err
	}
	return c.SetPassword(ctx, uid, hash)
}

// CreateGroup allocates a gidNumber (unless overridden) and creates the group.
func (s *Service) CreateGroup(ctx context.Context, c directory.Conn, cn string, gidOverride int) (*directory.Group, error) {
	if err := validName("cn", cn); err != nil {
		return nil, err
	}
	s.allocMu.Lock()
	defer s.allocMu.Unlock()
	gid := gidOverride
	if gid <= 0 {
		n, err := c.AllocateGIDNumber(ctx)
		if err != nil {
			return nil, err
		}
		gid = n
	}
	g := directory.Group{CN: cn, GIDNumber: gid}
	if err := c.CreateGroup(ctx, g); err != nil {
		return nil, err
	}
	return &g, nil
}

// normalizeMail drops a nil/empty mail profile.
func normalizeMail(m *directory.MailProfile) *directory.MailProfile {
	if m == nil {
		return nil
	}
	aliases := make([]string, 0, len(m.Aliases))
	for _, a := range m.Aliases {
		if a != "" {
			aliases = append(aliases, a)
		}
	}
	if m.Mail == "" && len(aliases) == 0 {
		return nil
	}
	return &directory.MailProfile{Mail: m.Mail, Aliases: aliases}
}
