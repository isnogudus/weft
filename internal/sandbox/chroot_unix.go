//go:build unix

package sandbox

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// chrootAndDrop performs the portable part of the sandbox, available on every
// Unix (Linux, macOS, the BSDs): when started as root it chroot(2)s and drops
// privileges to the configured user/group. It must be called only AFTER every
// required file has been read and the listening socket has been opened, because
// after chroot the original filesystem is no longer reachable and after the
// privilege drop root-owned files can no longer be opened.
//
// It returns whether a chroot was performed (so the caller can decide what is
// still reachable). If not started as root, the chroot/drop is skipped.
func chrootAndDrop(c Config) (chrooted bool, err error) {
	root := os.Geteuid() == 0
	drop := root && c.User != ""

	// Resolve the target uid/gid BEFORE chroot: user.Lookup reads /etc/passwd,
	// which is not present inside the chroot (e.g. /var/empty).
	var uid, gid int
	if drop {
		if uid, gid, err = lookupIDs(c.User, c.Group); err != nil {
			return false, err
		}
	}

	if root && c.Chroot != "" {
		if err := syscall.Chroot(c.Chroot); err != nil {
			return false, fmt.Errorf("chroot %q: %w", c.Chroot, err)
		}
		if err := syscall.Chdir("/"); err != nil {
			return false, fmt.Errorf("chdir /: %w", err)
		}
		chrooted = true
	}

	switch {
	case drop:
		// Order matters: drop supplementary groups and the gid before the uid,
		// since after setuid we no longer have the privilege to do so.
		if err := syscall.Setgroups([]int{gid}); err != nil {
			return chrooted, fmt.Errorf("setgroups: %w", err)
		}
		if err := syscall.Setgid(gid); err != nil {
			return chrooted, fmt.Errorf("setgid %d: %w", gid, err)
		}
		if err := syscall.Setuid(uid); err != nil {
			return chrooted, fmt.Errorf("setuid %d: %w", uid, err)
		}
		if os.Geteuid() == 0 {
			return chrooted, fmt.Errorf("sandbox: still root after privilege drop")
		}
		log.Printf("sandbox: dropped privileges to user %q (uid=%d gid=%d)%s",
			c.User, uid, gid, chrootNote(chrooted, c.Chroot))
	case root && c.Chroot != "":
		log.Print("sandbox: chrooted as root with no user to drop to (set 'user')")
	case !root && c.Chroot != "":
		log.Print("sandbox: not started as root -- skipping chroot/privilege drop")
	}

	return chrooted, nil
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
