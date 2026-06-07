// Package sandbox confines the privsep processes after startup. On OpenBSD it
// uses pledge(2)/unveil(2) and, when started as root, chroot(2) plus privilege
// dropping (ConfineWorker); the monitor is restricted with unveil + pledge
// (ConfineMonitor). On every other platform these are no-ops.
//
// The caller must have finished reading every file it needs (config, TLS
// certificates, CA bundle, system roots) and opened its listening socket
// BEFORE confining, because afterwards the filesystem and the set of permitted
// syscalls are restricted.
package sandbox

// Config describes the desired confinement. Paths/flags are derived by the
// caller from the resolved configuration.
type Config struct {
	Enabled    bool   // master switch
	Chroot     string // chroot dir; only used when running as root and non-empty
	User       string // user to drop to when chrooting
	Group      string // group to drop to ("" = the user's primary group)
	LDAPI      bool   // connecting to ldapd over a Unix socket
	SocketPath string // the ldapi socket path (when LDAPI)
	NeedsDNS   bool   // the LDAP host is a name that must be resolved at runtime
	Syslog     bool   // logging to syslog (the monitor needs /dev/log)
}
