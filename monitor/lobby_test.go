package monitor

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBattleLobbyPath(t *testing.T) {
	p := battleLobbyPath()
	if !strings.Contains(p, "Heroes of the Storm") {
		t.Fatalf("path missing game dir: %s", p)
	}
	if filepath.Base(p) != "replay.server.battlelobby" {
		t.Fatalf("unexpected file: %s", p)
	}
	if !strings.Contains(p, "TempWriteReplayP1") {
		t.Fatalf("path missing TempWriteReplayP1: %s", p)
	}
}

func TestIsFreshLobby(t *testing.T) {
	if isFreshLobby(time.Time{}, 90*time.Second) {
		t.Fatal("zero time must not be fresh")
	}
	if !isFreshLobby(time.Now().Add(-10*time.Second), 90*time.Second) {
		t.Fatal("10s ago should be fresh")
	}
	if isFreshLobby(time.Now().Add(-3*time.Hour), 90*time.Second) {
		t.Fatal("3h ago must be stale")
	}
}
