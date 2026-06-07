//go:build unix && !openbsd

package sandbox

import "log"

// Apply performs the portable chroot + privilege drop. pledge(2)/unveil(2) are
// OpenBSD-only, so on Linux, macOS, FreeBSD, NetBSD etc. the chroot and the
// privilege drop are the available confinement.
func Apply(c Config) error {
	if !c.Enabled {
		return nil
	}
	chrooted, err := chrootAndDrop(c)
	if err != nil {
		return err
	}
	if chrooted {
		log.Print("sandbox: pledge/unveil are OpenBSD-only; chroot + privilege drop applied")
	}
	return nil
}
