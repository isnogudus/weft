//go:build !unix

package applog

import "errors"

// Sink is a no-op stub on platforms without syslog (e.g. Windows).
type Sink struct{}

// NewSyslog is unsupported off Unix.
func NewSyslog(tag string) (*Sink, error) {
	return nil, errors.New("applog: syslog is not supported on this platform")
}

func (s *Sink) WriteLine(string)            {}
func (s *Sink) Write(p []byte) (int, error) { return len(p), nil }
func (s *Sink) Close() error                { return nil }
