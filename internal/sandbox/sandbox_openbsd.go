//go:build openbsd

package sandbox

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// Apply confines the current process. Sequence:
//  1. When started as root and Chroot is set: chroot(2) + chdir("/").
//  2. When started as root and User is set: drop to that user/group.
//  3. unveil(2): expose only the few runtime paths still needed, then lock
//     (skipped in the chroot case, where the chroot is already the FS sandbox).
//  4. pledge(2): reduce the permitted syscalls to the minimum.
func Apply(c Config) error {
	if !c.Enabled {
		return nil
	}
	root := os.Geteuid() == 0
	chrooted := false

	if root && c.Chroot != "" {
		if err := syscall.Chroot(c.Chroot); err != nil {
			return fmt.Errorf("chroot %q: %w", c.Chroot, err)
		}
		if err := syscall.Chdir("/"); err != nil {
			return fmt.Errorf("chdir /: %w", err)
		}
		chrooted = true
	}

	if root && c.User != "" {
		uid, gid, err := lookupIDs(c.User, c.Group)
		if err != nil {
			return err
		}
		if err := syscall.Setgroups([]int{gid}); err != nil {
			return fmt.Errorf("setgroups: %w", err)
		}
		if err := syscall.Setgid(gid); err != nil {
			return fmt.Errorf("setgid %d: %w", gid, err)
		}
		if err := syscall.Setuid(uid); err != nil {
			return fmt.Errorf("setuid %d: %w", uid, err)
		}
		if os.Geteuid() == 0 {
			return fmt.Errorf("sandbox: still root after privilege drop")
		}
		log.Printf("sandbox: dropped privileges to user %q (uid=%d gid=%d)%s",
			c.User, uid, gid, chrootNote(chrooted, c.Chroot))
	} else if root {
		log.Print("sandbox: running as root with no user to drop to (set 'user')")
	} else if c.Chroot != "" {
		log.Print("sandbox: not started as root -- skipping chroot/privilege drop")
	}

	// Filesystem confinement.
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

func lookupIDs(userName, groupName string) (uid, gid int, err error) {
	u, err := user.Lookup(userName)
	if err != nil {
		return 0, 0, fmt.Errorf("lookup user %q: %w", userName, err)
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("user %q has non-numeric uid %q", userName, u.Uid)
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("user %q has non-numeric gid %q", userName, u.Gid)
	}
	if groupName != "" {
		g, err := user.LookupGroup(groupName)
		if err != nil {
			return 0, 0, fmt.Errorf("lookup group %q: %w", groupName, err)
		}
		if gid, err = strconv.Atoi(g.Gid); err != nil {
			return 0, 0, fmt.Errorf("group %q has non-numeric gid %q", groupName, g.Gid)
		}
	}
	return uid, gid, nil
}

func chrootNote(chrooted bool, dir string) string {
	if chrooted {
		return fmt.Sprintf(", chrooted to %q", dir)
	}
	return ""
}
