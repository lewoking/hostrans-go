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

// isFreshLobby 启动时已存在的旧文件不算新对局（HotsStats 用 mtime 变化区分）。
func isFreshLobby(mt time.Time, maxAge time.Duration) bool {
	if mt.IsZero() {
		return false
	}
	age := time.Since(mt)
	return age >= 0 && age < maxAge
}
