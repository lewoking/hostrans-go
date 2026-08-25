package monitor

import (
	"reflect"
	"testing"
)

func TestMergeProbeHits_keepsIntersectionWhenLiveBufferExists(t *testing.T) {
	prev := []uintptr{0x1000, 0x2000, 0x3000}
	curr := []uintptr{0x2000, 0x9000}
	got := MergeProbeHits(prev, curr)
	want := []uintptr{0x2000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMergeProbeHits_fallsBackToLatestWhenAppendOnly(t *testing.T) {
	prev := []uintptr{0x1000, 0x2000}
	curr := []uintptr{0x8000, 0x9000}
	got := MergeProbeHits(prev, curr)
	want := []uintptr{0x8000, 0x9000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMergeProbeHits_emptyCurrentStaysEmpty(t *testing.T) {
	got := MergeProbeHits([]uintptr{0x1000}, nil)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}
