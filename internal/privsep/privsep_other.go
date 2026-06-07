//go:build !unix

// Stub for platforms without Unix fd passing (e.g. Windows). Privilege
// separation is unavailable there; the caller falls back to single-process mode.
package privsep

import (
	"errors"
	"net"
)

// RawDialer opens a raw connection to the LDAP endpoint.
type RawDialer func() (net.Conn, error)

// Supported reports whether privilege separation is available on this platform.
const Supported = false

// ErrUnsupported is returned by the privsep entry points on unsupported OSes.
var ErrUnsupported = errors.New("privsep: unsupported on this platform")

// IsWorker always reports false off Unix.
func IsWorker() bool { return false }

// Worker is a placeholder so callers compile on every platform.
type Worker struct {
	Listener net.Listener
}

// StartWorker is unsupported off Unix.
func StartWorker() (*Worker, error) { return nil, ErrUnsupported }

// Done returns a nil channel (never ready) off Unix.
func (w *Worker) Done() <-chan struct{} { return nil }

// DialLDAP is unsupported off Unix.
func (w *Worker) DialLDAP() (net.Conn, error) { return nil, ErrUnsupported }

// RunMonitor is unsupported off Unix.
func RunMonitor(ln *net.TCPListener, dial RawDialer, confine func() error) error {
	return ErrUnsupported
}
