// Package password produces userPassword values for the directory.
//
// weft only ever WRITES password hashes; it never verifies them. Verification
// happens on the LDAP side via passthrough bind. OpenBSD ldapd's crypt(3) is
// bcrypt, so we emit "{CRYPT}$2b$<cost>$..." which ldapd compares natively on
// bind. The cleartext password is hashed immediately and never logged.
package password

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// cryptPrefix is the RFC 2307 scheme marker ldapd recognises for crypt(3).
const cryptPrefix = "{CRYPT}"

// MaxBcryptLength is bcrypt's hard input limit; bytes beyond this are ignored,
// so we reject overlong passwords instead of silently truncating them.
const MaxBcryptLength = 72

// ErrTooLong is returned when the password exceeds the byte limit.
var ErrTooLong = errors.New("password: exceeds 72 bytes (bcrypt limit)")

// ErrEmpty is returned for an empty password.
var ErrEmpty = errors.New("password: must not be empty")

// Hash returns a userPassword value "{CRYPT}$2b$<cost>$..." for plain using the
// given bcrypt cost. It rejects empty or overlong passwords.
func Hash(plain string, cost int) (string, error) {
	if plain == "" {
		return "", ErrEmpty
	}
	if len(plain) > MaxBcryptLength {
		return "", ErrTooLong
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", fmt.Errorf("password: hashing failed: %w", err)
	}
	return cryptPrefix + string(h), nil
}
