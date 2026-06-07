package ldapd

import (
	"weft/internal/config"
	"weft/internal/directory"
)

// Compile-time assertions that the implementation satisfies the interfaces.
var _ directory.Directory = New(config.Default())
var _ directory.Conn = (*conn)(nil)
