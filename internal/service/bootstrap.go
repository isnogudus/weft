package service

import (
	"context"
	"errors"

	"weft/internal/directory"
)

// Bootstrap creates the base structure on a fresh directory: ou=people,
// ou=groups and the default primary group. It must be called with an admin
// (rootdn) connection. It is idempotent: entries that already exist are
// skipped, so re-running the wizard is safe.
//
// No first admin user is created: the admin is the synthetic rootdn, so it
// needs no directory entry.
func (s *Service) Bootstrap(ctx context.Context, c directory.Conn) error {
	if !c.IsAdmin() {
		return errors.New("bootstrap requires the admin (rootdn) connection")
	}
	// The base/suffix entry must exist before any child can be added (ldapd
	// does not auto-create it).
	if err := skipExists(c.CreateBaseDN(ctx)); err != nil {
		return err
	}
	if err := skipExists(c.CreateOU(ctx, s.cfg.PeopleOU)); err != nil {
		return err
	}
	if err := skipExists(c.CreateOU(ctx, s.cfg.GroupsOU)); err != nil {
		return err
	}
	// Default primary group, with an allocated gidNumber.
	if _, err := c.GetGroup(ctx, s.cfg.PrimaryGroup); err != nil {
		if !errors.Is(err, directory.ErrNotFound) {
			return err
		}
		if _, err := s.CreateGroup(ctx, c, s.cfg.PrimaryGroup, 0); err != nil {
			return err
		}
	}
	return nil
}

func skipExists(err error) error {
	if errors.Is(err, directory.ErrAlreadyExists) {
		return nil
	}
	return err
}
