//go:build !windows

package memory

func IsAdmin() bool { return false }

func ElevateIfNeeded() {}

func EnsureSingleInstance() error { return nil }
