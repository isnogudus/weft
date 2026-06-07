//go:build unix && !openbsd

package sandbox

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
