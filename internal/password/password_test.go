package password

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHash(t *testing.T) {
	h, err := Hash("correct horse battery staple", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "{CRYPT}$2") {
		t.Fatalf("missing {CRYPT}$2 prefix: %q", h)
	}
	// The bcrypt portion must verify against the original cleartext, proving
	// ldapd (which uses crypt(3)=bcrypt) will accept it on bind.
	raw := strings.TrimPrefix(h, "{CRYPT}")
	if err := bcrypt.CompareHashAndPassword([]byte(raw), []byte("correct horse battery staple")); err != nil {
		t.Fatalf("hash does not verify: %v", err)
	}
}

func TestHashRejects(t *testing.T) {
	if _, err := Hash("", 4); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty: got %v", err)
	}
	if _, err := Hash(strings.Repeat("x", 73), 4); !errors.Is(err, ErrTooLong) {
		t.Fatalf("too long: got %v", err)
	}
}
