//go:build unix

// Package privsep implements OpenBSD-style privilege separation: a small
// privileged monitor process opens connections to the LDAP server (DNS +
// connect, for TCP or the ldapi Unix socket) and passes the connected file
// descriptors to an unprivileged, chrooted worker process over a socketpair
// using SCM_RIGHTS. The worker speaks HTTP and LDAP but never needs filesystem
// access to resolv.conf, the ldapi socket, or anything outside its chroot.
//
// fd passing is a plain Unix mechanism (socketpair(2) + SCM_RIGHTS), so this
// works on Linux, macOS and the BSDs alike; only pledge(2)/unveil(2) (applied
// elsewhere) are OpenBSD-specific.
package privsep

import (
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// socketpair returns a connected pair of stream sockets as *os.File. Each end
// can be turned into a *net.UnixConn with net.FileConn, or inherited by a child
// via exec.Cmd.ExtraFiles.
func socketpair() (a, b *os.File, err error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("socketpair: %w", err)
	}
	return os.NewFile(uintptr(fds[0]), "privsep-a"), os.NewFile(uintptr(fds[1]), "privsep-b"), nil
}

// unixConn turns an *os.File socket into a *net.UnixConn (and closes the file,
// since net.FileConn dups the descriptor).
func unixConn(f *os.File) (*net.UnixConn, error) {
	c, err := net.FileConn(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	uc, ok := c.(*net.UnixConn)
	if !ok {
		c.Close()
		return nil, fmt.Errorf("privsep: expected *net.UnixConn, got %T", c)
	}
	return uc, nil
}

// sendFD sends a single open file descriptor over c, with a one-byte payload
// carrying a status code (0 = ok). The descriptor is duplicated into the peer.
func sendFD(c *net.UnixConn, status byte, fd int) error {
	rights := unix.UnixRights(fd)
	_, _, err := c.WriteMsgUnix([]byte{status}, rights, nil)
	return err
}

// sendStatus sends a one-byte status with no descriptor (used for errors).
func sendStatus(c *net.UnixConn, status byte) error {
	_, _, err := c.WriteMsgUnix([]byte{status}, nil, nil)
	return err
}

// recvFD receives a one-byte status and, when present, a single file
// descriptor. It returns the status byte and the received fd (-1 if none).
func recvFD(c *net.UnixConn) (status byte, fd int, err error) {
	payload := make([]byte, 1)
	oob := make([]byte, unix.CmsgSpace(4)) // room for exactly one fd
	n, oobn, _, _, err := c.ReadMsgUnix(payload, oob)
	if err != nil {
		return 0, -1, err
	}
	if n < 1 {
		return 0, -1, errors.New("privsep: short control message")
	}
	status = payload[0]
	if oobn == 0 {
		return status, -1, nil
	}
	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return status, -1, fmt.Errorf("privsep: parse scm: %w", err)
	}
	for _, m := range msgs {
		fds, err := unix.ParseUnixRights(&m)
		if err != nil {
			return status, -1, fmt.Errorf("privsep: parse rights: %w", err)
		}
		if len(fds) > 0 {
			// Close any extra fds we did not expect.
			for _, extra := range fds[1:] {
				unix.Close(extra)
			}
			return status, fds[0], nil
		}
	}
	return status, -1, nil
}
