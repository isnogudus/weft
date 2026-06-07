//go:build unix

package privsep

import (
	"io"
	"os"
	"testing"
)

func TestFDPassing(t *testing.T) {
	fa, fb, err := socketpair()
	if err != nil {
		t.Fatal(err)
	}
	ca, err := unixConn(fa)
	if err != nil {
		t.Fatal(err)
	}
	defer ca.Close()
	cb, err := unixConn(fb)
	if err != nil {
		t.Fatal(err)
	}
	defer cb.Close()

	// The "monitor" passes the write end of a pipe to the "worker".
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()

	if err := sendFD(ca, 0, int(pw.Fd())); err != nil {
		t.Fatal(err)
	}
	pw.Close() // the peer holds a dup now

	status, fd, err := recvFD(cb)
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 || fd < 0 {
		t.Fatalf("status=%d fd=%d", status, fd)
	}

	// Writing to the received fd must reach the original pipe's read end,
	// proving it is the same kernel file object passed across the socketpair.
	recv := os.NewFile(uintptr(fd), "recv")
	if _, err := recv.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	recv.Close()

	buf := make([]byte, 5)
	if _, err := io.ReadFull(pr, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello" {
		t.Fatalf("got %q, want hello", buf)
	}
}

func TestRecvFDErrorStatus(t *testing.T) {
	fa, fb, err := socketpair()
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := unixConn(fa)
	cb, _ := unixConn(fb)
	defer ca.Close()
	defer cb.Close()

	if err := sendStatus(ca, 1); err != nil {
		t.Fatal(err)
	}
	status, fd, err := recvFD(cb)
	if err != nil {
		t.Fatal(err)
	}
	if status != 1 || fd != -1 {
		t.Fatalf("status=%d fd=%d, want 1/-1", status, fd)
	}
}
