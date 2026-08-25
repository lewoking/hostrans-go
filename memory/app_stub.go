//go:build !windows

package memory

func IsAdmin() bool { return false }

func ElevateIfNeeded() {}

func EnsureSingleInstance() error { return nil }

func CreateQuitEvent() (QuitHandle, error) { return 0, nil }

func CloseQuitEvent(QuitHandle) {}

func RequestQuit() error { return nil }

func WaitQuit(QuitHandle, func()) {}
