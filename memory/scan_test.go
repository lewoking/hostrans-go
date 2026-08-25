package memory

import "testing"

func TestShouldScanRegion_includesMappedAndLargeWritable(t *testing.T) {
	cases := []struct {
		name    string
		state   uint32
		typ     uint32
		protect uint32
		size    uintptr
	}{
		{"private RW 8MB", memCommit, memPrivate, pageReadWrite, 8 << 20},
		{"private RW 200MB", memCommit, memPrivate, pageReadWrite, 200 << 20},
		{"mapped RW", memCommit, memMapped, pageReadWrite, 4 << 20},
		{"private ERW", memCommit, memPrivate, pageExecuteReadWrite, 1 << 20},
		{"private writecopy", memCommit, memPrivate, pageWriteCopy, 1 << 20},
	}
	for _, c := range cases {
		if !ShouldScanRegion(c.state, c.typ, c.protect, c.size) {
			t.Errorf("%s: expected scan", c.name)
		}
	}
}

func TestShouldScanRegion_skipsUselessRegions(t *testing.T) {
	cases := []struct {
		name    string
		state   uint32
		typ     uint32
		protect uint32
		size    uintptr
	}{
		{"reserved", 0x2000, memPrivate, pageReadWrite, 1 << 20},
		{"readonly", memCommit, memPrivate, 0x02, 1 << 20},
		{"guard", memCommit, memPrivate, pageReadWrite | pageGuard, 1 << 20},
		{"empty", memCommit, memPrivate, pageReadWrite, 0},
		{"image", memCommit, 0x1000000, pageReadWrite, 1 << 20},
	}
	for _, c := range cases {
		if ShouldScanRegion(c.state, c.typ, c.protect, c.size) {
			t.Errorf("%s: should skip", c.name)
		}
	}
}
