package dlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	LevelDebug = 0
	LevelInfo  = 1
)

// Version 由 -ldflags "-X hostrans/dlog.Version=v0.5.4" 注入；空或 dev 视为 1.0 前。
var Version = "dev"

var (
	mu     sync.Mutex
	path   string
	trunc  = true
)

func Path() string {
	initFile()
	return path
}

func DebugEnabled(ver string) bool {
	s := strings.TrimSpace(ver)
	if s == "" || s == "dev" {
		return true
	}
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexAny(s, ".-+"); i >= 0 {
		// keep first number even if 0.5.3
	}
	parts := strings.SplitN(s, ".", 2)
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return true
	}
	return maj < 1
}

func minLevel() int {
	if DebugEnabled(Version) {
		return LevelDebug
	}
	return LevelInfo
}

func initFile() {
	mu.Lock()
	defer mu.Unlock()
	if path != "" {
		return
	}
	path = filepath.Join(os.TempDir(), "hostrans.log")
}

func write(level, format string, args ...interface{}) {
	initFile()
	mu.Lock()
	defer mu.Unlock()
	flags := os.O_CREATE | os.O_WRONLY
	if trunc {
		flags |= os.O_TRUNC
		trunc = false
	} else {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+"["+level+"] "+format+"\n", args...)
}

func Debugf(format string, args ...interface{}) {
	if minLevel() > LevelDebug {
		return
	}
	write("DBG", format, args...)
}

func Infof(format string, args ...interface{}) {
	write("INF", format, args...)
}

// Printf 兼容旧调用：1.0 前当 debug，1.0 起不再刷屏。
func Printf(format string, args ...interface{}) {
	Debugf(format, args...)
}
