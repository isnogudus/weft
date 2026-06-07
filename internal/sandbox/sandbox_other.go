//go:build !openbsd

package sandbox

import "log"

// Apply is a no-op on non-OpenBSD platforms (used for development on other OSes).
func Apply(c Config) error {
	if c.Enabled {
		log.Print("sandbox: pledge/unveil/chroot are OpenBSD-only; running without OS sandboxing")
	}
	return nil
}
