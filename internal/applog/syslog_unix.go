//go:build unix

// Package applog provides an optional syslog sink for weft's logs. Under privsep
// it is owned by the (non-chrooted) monitor, which can reach /dev/log and
// reconnect across syslogd restarts; the chrooted worker keeps logging to
// stderr, which the monitor captures and forwards here.
package applog

import (
	"fmt"
	"log/syslog"
	"os"
	"strings"
)

// Sink writes lines to the local syslog (facility LOG_DAEMON, severity INFO).
// Go's syslog.Writer reconnects internally on a write error, so a syslogd
// restart is handled transparently; if syslog is entirely unreachable the line
// falls back to stderr so it is not silently lost.
type Sink struct {
	w *syslog.Writer
}

// NewSyslog opens a syslog connection with the given program tag.
func NewSyslog(tag string) (*Sink, error) {
	w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, tag)
	if err != nil {
		return nil, err
	}
	return &Sink{w: w}, nil
}

// WriteLine logs a single line.
func (s *Sink) WriteLine(line string) {
	line = strings.TrimRight(line, "\n")
	if line == "" {
		return
	}
	if err := s.w.Info(line); err != nil {
		fmt.Fprintln(os.Stderr, line)
	}
}

// Write implements io.Writer so the sink can back the standard log package.
func (s *Sink) Write(p []byte) (int, error) {
	s.WriteLine(string(p))
	return len(p), nil
}

// Close closes the syslog connection.
func (s *Sink) Close() error { return s.w.Close() }
