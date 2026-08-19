package applog

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

// Level is the severity threshold for weft's own log output. weft logs little
// and logs it once: lifecycle events and one line per HTTP request at Info,
// problems at Warn/Error, and the individual LDAP operations at Debug (never
// credentials -- see the ldapclient package).
type Level int32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Line prefixes. Info -- the ordinary case -- is unprefixed, so the log format
// operators are used to reading does not change. The prefixes are also how the
// syslog sink recovers a line's severity: under privilege separation the
// monitor forwards the worker's stderr verbatim, so the text is all it has.
const (
	prefixDebug = "debug: "
	prefixWarn  = "warning: "
	prefixError = "error: "
)

// current is the active threshold, read on every log call from any goroutine.
var current atomic.Int32

// ParseLevel maps a configuration string to a Level.
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "", "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	}
	return LevelInfo, fmt.Errorf("applog: unknown log level %q (debug, info, warn, error)", s)
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// SetLevel sets the threshold; messages below it are dropped at the call site.
// Both privsep processes set it from the same configuration.
func SetLevel(l Level) { current.Store(int32(l)) }

// CurrentLevel returns the active threshold.
func CurrentLevel() Level { return Level(current.Load()) }

// Enabled reports whether messages of level l are logged. Use it to skip work
// that only exists to produce a log line.
func Enabled(l Level) bool { return l >= CurrentLevel() }

// Debugf logs one LDAP/internal detail line, off unless log_level = "debug".
func Debugf(format string, args ...any) { emit(LevelDebug, prefixDebug, format, args) }

// Infof logs a lifecycle event or a served request.
func Infof(format string, args ...any) { emit(LevelInfo, "", format, args) }

// Warnf logs something the operator should look at, but that does not stop weft.
func Warnf(format string, args ...any) { emit(LevelWarn, prefixWarn, format, args) }

// Errorf logs a failure. weft has no fatal logger: startup failures are
// returned as errors and printed by main.
func Errorf(format string, args ...any) { emit(LevelError, prefixError, format, args) }

func emit(l Level, prefix, format string, args []any) {
	if !Enabled(l) {
		return
	}
	log.Print(prefix, fmt.Sprintf(format, args...))
}
