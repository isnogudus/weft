//go:build openbsd

package sandbox

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// ConfineWorker confines the privsep worker: chroot + privilege drop, then
// pledge to a minimal set. The worker never opens LDAP connections itself (the
// monitor passes descriptors), so it needs no DNS, no outbound dialing and no
// filesystem -- "stdio inet recvfd" (inet to accept on the HTTP listener,
// recvfd to receive passed descriptors).
func ConfineWorker(c Config) error {
	if !c.Enabled {
		return nil
	}
	if _, err := chrootAndDrop(c); err != nil {
		return err
	}
	if err := unix.UnveilBlock(); err != nil {
		return fmt.Errorf("unveil lock: %w", err)
	}
	// The worker opens no files at runtime (the caller warms every lazy
	// filesystem init -- MIME table, timezone, CSPRNG -- beforehand), so it does
	// not promise rpath. A stray open() here is a bug to find and warm, not to
	// permit.
	const promises = "stdio inet recvfd"
	if err := unix.PledgePromises(promises); err != nil {
		return fmt.Errorf("pledge %q: %w", promises, err)
	}
	log.Printf("sandbox: worker pledge(%q); filesystem locked", promises)
	return nil
}

// ConfineMonitor confines the privsep monitor: it stays privileged (it opens
// LDAP connections) but its filesystem view is restricted with unveil(2) to the
// few paths it needs to dial and log, then it is pledged. It stops the worker by
// closing a shutdown pipe rather than kill(2), so it needs no "proc".
func ConfineMonitor(c Config) error {
	if !c.Enabled {
		return nil
	}
	// Expose only what the monitor actually opens, then lock the filesystem.
	if c.NeedsDNS {
		unveilIfExists("/etc/resolv.conf", "r")
		unveilIfExists("/etc/hosts", "r")
	}
	if c.LDAPI && c.SocketPath != "" {
		if err := unix.Unveil(c.SocketPath, "rw"); err != nil {
			return fmt.Errorf("unveil %q: %w", c.SocketPath, err)
		}
	}
	if c.Syslog {
		unveilIfExists("/dev/log", "rw")
	}
	if err := unix.UnveilBlock(); err != nil {
		return fmt.Errorf("unveil lock: %w", err)
	}

	const promises = "stdio rpath inet dns unix sendfd"
	if err := unix.PledgePromises(promises); err != nil {
		return fmt.Errorf("pledge %q: %w", promises, err)
	}
	log.Printf("sandbox: monitor pledge(%q); filesystem restricted", promises)
	return nil
}

// unveilIfExists unveils a path, ignoring it if it does not exist.
func unveilIfExists(path, perms string) {
	if err := unix.Unveil(path, perms); err != nil && !os.IsNotExist(err) {
		log.Printf("sandbox: unveil %q: %v", path, err)
	}
}
