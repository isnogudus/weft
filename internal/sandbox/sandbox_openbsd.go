//go:build openbsd

package sandbox

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// Apply confines the process: the portable chroot + privilege drop, then the
// OpenBSD-specific unveil(2)/pledge(2). Call it only after every required file
// has been read and the listener opened.
func Apply(c Config) error {
	if !c.Enabled {
		return nil
	}
	chrooted, err := chrootAndDrop(c)
	if err != nil {
		return err
	}

	// Filesystem confinement. Under chroot the filesystem is already the
	// (empty) chroot, so we only need to lock further unveils; otherwise expose
	// the few runtime paths first.
	if !chrooted {
		for path, perms := range runtimePaths(c) {
			if err := unix.Unveil(path, perms); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return fmt.Errorf("unveil %q: %w", path, err)
			}
		}
	}
	if err := unix.UnveilBlock(); err != nil {
		return fmt.Errorf("unveil lock: %w", err)
	}

	// Syscall confinement.
	promises := pledgePromises(c)
	if err := unix.PledgePromises(promises); err != nil {
		return fmt.Errorf("pledge %q: %w", promises, err)
	}
	log.Printf("sandbox: pledge(%q); filesystem locked", promises)
	return nil
}

// runtimePaths returns the host paths weft still needs to read after startup
// (only used when NOT chrooted). Everything else was read or warmed already.
func runtimePaths(c Config) map[string]string {
	paths := map[string]string{}
	if c.LDAPI {
		if c.SocketPath != "" {
			paths[c.SocketPath] = "rw"
		}
		return paths
	}
	if c.NeedsDNS {
		paths["/etc/resolv.conf"] = "r"
		paths["/etc/hosts"] = "r"
	}
	// System trust store / configured CA (defensive; usually already cached).
	paths["/etc/ssl/cert.pem"] = "r"
	if c.CACertFile != "" {
		paths[c.CACertFile] = "r"
	}
	return paths
}

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
	const promises = "stdio inet recvfd"
	if err := unix.PledgePromises(promises); err != nil {
		return fmt.Errorf("pledge %q: %w", promises, err)
	}
	log.Printf("sandbox: worker pledge(%q); filesystem locked", promises)
	return nil
}

// ConfineMonitor confines the privsep monitor: it stays privileged (it opens
// LDAP connections) but is pledged to just dial + pass descriptors.
func ConfineMonitor(c Config) error {
	if !c.Enabled {
		return nil
	}
	const promises = "stdio rpath inet dns unix sendfd"
	if err := unix.PledgePromises(promises); err != nil {
		return fmt.Errorf("pledge %q: %w", promises, err)
	}
	log.Printf("sandbox: monitor pledge(%q)", promises)
	return nil
}

func pledgePromises(c Config) string {
	p := "stdio rpath inet"
	if c.NeedsDNS {
		p += " dns"
	}
	if c.LDAPI {
		p += " unix"
	}
	return p
}
