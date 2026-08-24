package monitor

import (
	"path/filepath"
	"strings"
	"testing"
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
