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

// ConfineWorker confines the privsep worker (chroot + privilege drop; pledge is
// OpenBSD-only).
func ConfineWorker(c Config) error {
	if !c.Enabled {
		return nil
	}
	_, err := chrootAndDrop(c)
	return err
}

// ConfineMonitor is a no-op off OpenBSD (the monitor keeps its privileges to
// open connections; there is no pledge here).
func ConfineMonitor(c Config) error { return nil }
