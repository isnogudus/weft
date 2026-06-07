//go:build !unix

package sandbox

import "log"

// Apply is a no-op on non-Unix platforms (e.g. Windows): chroot and privilege
// dropping are not available there.
func Apply(c Config) error {
	if c.Enabled {
		log.Print("sandbox: OS sandboxing is not available on this platform")
	}
	return nil
}
