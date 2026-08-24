package monitor

import (
	"os"
	"path/filepath"
	"time"
)

// HotsStats 同源：加载画面开始时游戏会写出 replay.server.battlelobby。
// 路径：%TEMP%\Heroes of the Storm\TempWriteReplayP1\replay.server.battlelobby
func battleLobbyPath() string {
	return filepath.Join(os.TempDir(), "Heroes of the Storm", "TempWriteReplayP1", "replay.server.battlelobby")
}

func battleLobbyModTime() (time.Time, bool) {
	fi, err := os.Stat(battleLobbyPath())
	if err != nil {
		return time.Time{}, false
	}
	return fi.ModTime(), true
}
