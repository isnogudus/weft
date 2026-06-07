//go:build !unix

package sandbox

// ConfineWorker is a no-op on non-Unix platforms.
func ConfineWorker(c Config) error { return nil }

// ConfineMonitor is a no-op on non-Unix platforms.
func ConfineMonitor(c Config) error { return nil }
