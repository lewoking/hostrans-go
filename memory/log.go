package memory

import "hostrans/dlog"

func debugMem(format string, args ...interface{}) {
	dlog.Printf("mem: "+format, args...)
}
