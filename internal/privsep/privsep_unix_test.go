//go:build unix

package privsep

import (
	"errors"
	"io"
	"net"
	"testing"
)

// echoServer accepts connections and echoes bytes back.
func echoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go io.Copy(c, c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestDialProtocol(t *testing.T) {
	addr, stop := echoServer(t)
	defer stop()

	fa, fb, err := socketpair()
	if err != nil {
		t.Fatal(err)
	}
	mc, _ := unixConn(fa)
	wc, _ := unixConn(fb)
	defer wc.Close()

	go serveDials(mc, func() (net.Conn, error) { return net.Dial("tcp", addr) })
	w := &Worker{ctrl: wc}

	// Two separate requests prove the monitor re-dials on demand and each
	// passed fd is an independent, working connection to the echo server.
	for _, msg := range []string{"ping", "pong"} {
		conn, err := w.DialLDAP()
		if err != nil {
			t.Fatalf("DialLDAP: %v", err)
		}
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, len(msg))
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Fatal(err)
		}
		if string(buf) != msg {
			t.Fatalf("echo = %q, want %q", buf, msg)
		}
		conn.Close()
	}
}

func TestDialProtocolError(t *testing.T) {
	fa, fb, err := socketpair()
	if err != nil {
		t.Fatal(err)
	}
	mc, _ := unixConn(fa)
	wc, _ := unixConn(fb)
	defer wc.Close()

	go serveDials(mc, func() (net.Conn, error) { return nil, errors.New("unreachable") })
	w := &Worker{ctrl: wc}

	if _, err := w.DialLDAP(); err == nil {
		t.Fatal("expected error when the monitor cannot dial")
	}
}
