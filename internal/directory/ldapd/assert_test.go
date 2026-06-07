package ldapd

import (
	"weft/internal/directory"
)

// Compile-time assertions that the implementation satisfies the interfaces.
var _ directory.Directory = (*Directory)(nil)
var _ directory.Conn = (*conn)(nil)
