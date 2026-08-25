package monitor

import "testing"

func TestEmitSkipsGarbageAndKeepsChat(t *testing.T) {
	m := New(nil)
	m.emit(string([]byte{0xaa, 0xa7, 0x31, 0x88, 0xaa, 0xaa, 0xaa}), "", nil, nil)
	select {
	case j := <-m.jobs:
		t.Fatalf("queued garbage %q", j.body)
	default:
	}
	m.emit("[团队] honghuli: 이길 수 없다", "", nil, nil)
	select {
	case j := <-m.jobs:
		if j.speaker != "honghuli" || j.body != "이길 수 없다" {
			t.Fatalf("got %+v", j)
		}
	default:
		t.Fatal("expected korean chat")
	}
	m.emit("[团队]: 대왕이 왔어", "", nil, nil)
	select {
	case j := <-m.jobs:
		if j.body != "대왕이 왔어" {
			t.Fatalf("got %+v", j)
		}
	default:
		t.Fatal("expected second korean chat")
	}
	m.emit("이 왔어", "", nil, nil)
	select {
	case j := <-m.jobs:
		t.Fatalf("queued fragment %q", j.body)
	default:
	}
}
