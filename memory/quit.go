package memory

import (
	"os"
	"strings"
)

// QuitHandle 进程退出事件。Windows 上是 Event 句柄。
type QuitHandle uintptr

func WantQuit() bool {
	for _, a := range os.Args[1:] {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "--quit", "-quit", "/quit", "-q":
			return true
		}
	}
	return false
}
