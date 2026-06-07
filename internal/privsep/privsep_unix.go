//go:build unix

package privsep

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

// Inherited descriptor numbers in the worker process. exec.Cmd.ExtraFiles maps
// the i-th extra file to fd 3+i.
const (
	fdControl  = 3 // socketpair end to the monitor
	fdListener = 4 // the HTTP listening socket
	workerEnv  = "WEFT_PRIVSEP_WORKER"
)

// request/status bytes exchanged over the control socket.
const (
	reqDial   byte = 1 // worker -> monitor: "open an LDAP connection"
	statusOK  byte = 0 // monitor -> worker: fd follows
	statusErr byte = 1 // monitor -> worker: dial failed, no fd
)

// Supported reports whether privilege separation is available on this platform.
const Supported = true

// IsWorker reports whether this process is the re-exec'd, unprivileged worker.
func IsWorker() bool { return os.Getenv(workerEnv) == "1" }

// RawDialer opens a fresh, un-TLS'd connection to the LDAP endpoint (DNS +
// connect, TCP or the ldapi Unix socket). The monitor runs it on behalf of the
// chrooted worker.
type RawDialer func() (net.Conn, error)

// --- monitor side ---

// RunMonitor re-execs this binary as the worker, handing it the HTTP listener
// and one end of a control socketpair, then serves the worker's dial requests
// (one passed fd per request) until the worker exits. ln is the raw TCP
// listener; the worker wraps it in TLS itself if configured. confine, if
// non-nil, is called after the worker is started to confine the monitor itself
// (e.g. pledge); terminating signals are forwarded to the worker.
func RunMonitor(ln *net.TCPListener, dial RawDialer, confine func() error) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("privsep: locate executable: %w", err)
	}
	monFile, workerCtrl, err := socketpair()
	if err != nil {
		return err
	}
	defer monFile.Close()

	lnFile, err := ln.File() // a dup of the listener fd for the child
	if err != nil {
		workerCtrl.Close()
		return fmt.Errorf("privsep: listener fd: %w", err)
	}

	cmd := exec.Command(self, os.Args[1:]...)
	cmd.Env = append(os.Environ(), workerEnv+"=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.ExtraFiles = []*os.File{workerCtrl, lnFile} // -> fd 3, fd 4
	if err := cmd.Start(); err != nil {
		workerCtrl.Close()
		lnFile.Close()
		return fmt.Errorf("privsep: start worker: %w", err)
	}
	// The child inherited dups; the monitor drops its own copies.
	workerCtrl.Close()
	lnFile.Close()

	// Forward terminating signals to the worker so it can shut down gracefully.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		if s, ok := <-sigCh; ok {
			_ = cmd.Process.Signal(s)
		}
	}()

	mc, err := unixConn(monFile)
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	if confine != nil {
		if err := confine(); err != nil {
			_ = cmd.Process.Kill()
			return fmt.Errorf("privsep: confine monitor: %w", err)
		}
	}
	go serveDials(mc, dial)

	return cmd.Wait()
}

// serveDials answers each dial request by opening a connection and passing its
// fd to the worker. It returns when the control socket closes (worker exit).
func serveDials(mc *net.UnixConn, dial RawDialer) {
	defer mc.Close()
	for {
		_, _, err := recvFD(mc) // one request byte (carries no fd)
		if err != nil {
			return // worker gone
		}
		conn, derr := dial()
		if derr != nil {
			_ = sendStatus(mc, statusErr)
			continue
		}
		f, ferr := connFile(conn)
		if ferr != nil {
			conn.Close()
			_ = sendStatus(mc, statusErr)
			continue
		}
		if err := sendFD(mc, statusOK, int(f.Fd())); err != nil {
			f.Close()
			conn.Close()
			return
		}
		f.Close()    // monitor drops its dup
		conn.Close() // the worker holds a dup of the socket
	}
}

// connFile extracts a dup'd *os.File from a TCP or Unix connection.
func connFile(c net.Conn) (*os.File, error) {
	type filer interface{ File() (*os.File, error) }
	if f, ok := c.(filer); ok {
		return f.File()
	}
	return nil, fmt.Errorf("privsep: connection type %T cannot pass a descriptor", c)
}

// --- worker side ---

// Worker is the unprivileged side: it owns the inherited HTTP listener and the
// control socket, and asks the monitor for LDAP connections.
type Worker struct {
	Listener net.Listener
	ctrl     *net.UnixConn
	mu       sync.Mutex
}

// StartWorker reconstructs the inherited descriptors. Call it once, early, in
// the worker process (when IsWorker() is true).
func StartWorker() (*Worker, error) {
	ctrl, err := unixConn(os.NewFile(fdControl, "privsep-control"))
	if err != nil {
		return nil, fmt.Errorf("privsep: control fd: %w", err)
	}
	lnFile := os.NewFile(fdListener, "privsep-listener")
	ln, err := net.FileListener(lnFile)
	lnFile.Close()
	if err != nil {
		return nil, fmt.Errorf("privsep: listener fd: %w", err)
	}
	return &Worker{Listener: ln, ctrl: ctrl}, nil
}

// DialLDAP asks the monitor to open a connection to the LDAP endpoint and
// returns it. It is safe for concurrent use (requests are serialised).
func (w *Worker) DialLDAP() (net.Conn, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := sendStatus(w.ctrl, reqDial); err != nil {
		return nil, fmt.Errorf("privsep: request dial: %w", err)
	}
	status, fd, err := recvFD(w.ctrl)
	if err != nil {
		return nil, fmt.Errorf("privsep: receive fd: %w", err)
	}
	if status != statusOK || fd < 0 {
		return nil, fmt.Errorf("privsep: monitor could not reach the LDAP server")
	}
	f := os.NewFile(uintptr(fd), "ldap")
	conn, err := net.FileConn(f)
	f.Close()
	return conn, err
}
