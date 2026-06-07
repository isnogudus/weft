package service

import (
	"fmt"
	"regexp"
	"strings"
)

// uidPattern restricts uids and group names to a safe POSIX-ish charset. This
// also prevents DN injection, since uids/cns are interpolated into DNs by the
// directory layer's templates.
var namePattern = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`)

// ValidName reports whether s is an acceptable uid or group cn.
func ValidName(s string) bool { return namePattern.MatchString(s) }

func validName(field, s string) error {
	if !ValidName(s) {
		return fmt.Errorf("%s %q is invalid (allowed: lowercase letter/underscore start, then [a-z0-9_.-], max 32)", field, s)
	}
	return nil
}

// validText rejects empty or control-character-bearing free-text fields.
func validText(field, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	for _, r := range s {
		if r < 0x20 {
			return fmt.Errorf("%s contains control characters", field)
		}
	}
	return nil
}
