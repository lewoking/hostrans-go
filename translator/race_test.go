package translator

import (
	"errors"
	"testing"
	"time"
)

type stubEngine struct {
	name  string
	delay time.Duration
	out   string
	err   error
}

func (s stubEngine) Name() string { return s.name }

func (s stubEngine) Translate(text, from, to string) (string, error) {
	time.Sleep(s.delay)
	return s.out, s.err
}

func testManager(engines ...Translator) *Manager {
	return &Manager{engines: engines, cache: make(map[string]string)}
}

func TestTranslateRace_firstValidWins(t *testing.T) {
	m := testManager(
		stubEngine{name: "slowOK", delay: 80 * time.Millisecond, out: "大王来了"},
		stubEngine{name: "fastOK", delay: 10 * time.Millisecond, out: "国王来了"},
	)
	start := time.Now()
	got, err := m.Translate("대왕이 왔어", "ko", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if got != "国王来了" {
		t.Fatalf("got %q, want first valid 国王来了", got)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("should not wait for slow engine, took %s", time.Since(start))
	}
}

func TestTranslateRace_skipRejectWaitForValid(t *testing.T) {
	m := testManager(
		stubEngine{name: "fastReject", delay: 10 * time.Millisecond, out: "대왕이 왔어"},
		stubEngine{name: "slowOK", delay: 40 * time.Millisecond, out: "大王来了"},
	)
	got, err := m.Translate("대왕이 왔어", "ko", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if got != "大王来了" {
		t.Fatalf("got %q, reject should not win", got)
	}
}

func TestTranslateRace_allFail(t *testing.T) {
	m := testManager(
		stubEngine{name: "a", delay: 5 * time.Millisecond, err: errors.New("timeout")},
		stubEngine{name: "b", delay: 5 * time.Millisecond, out: "대왕이 왔어"},
	)
	if _, err := m.Translate("대왕이 왔어", "ko", "zh"); err == nil {
		t.Fatal("expected failure")
	}
}
