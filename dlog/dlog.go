package dlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu   sync.Mutex
	path string
)

func Path() string {
	initFile()
	return path
}

func initFile() {
	mu.Lock()
	defer mu.Unlock()
	if path != "" {
		return
	}
	path = filepath.Join(os.TempDir(), "hostrans.log")
}

func Printf(format string, args ...interface{}) {
	initFile()
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}
