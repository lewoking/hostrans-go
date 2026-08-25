package dlog

import "testing"

func TestDebugEnabledBeforeV1(t *testing.T) {
	on := []string{"", "dev", "0.5.3", "v0.5.3", "v0.9.9", "0.1.0-rc1"}
	for _, v := range on {
		if !DebugEnabled(v) {
			t.Errorf("version %q should enable debug", v)
		}
	}
	off := []string{"1.0.0", "v1.0.0", "1.0.1", "v2.0.0"}
	for _, v := range off {
		if DebugEnabled(v) {
			t.Errorf("version %q should not enable debug", v)
		}
	}
}
