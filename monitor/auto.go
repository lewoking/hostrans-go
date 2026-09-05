package monitor

import (
	"fmt"
	"strings"
	"time"

	"hostrans/dlog"
	"hostrans/memory"
)

var chatMarkers = []string{
	`<c val="3184FF">[团队]:</c>`,
	`<c val="3184FF">[队伍]:</c>`,
	`[团队]:`,
	`[队伍]:`,
	`[组队]:`,
	`团队]`,
	`[征召团队]`,
	`[房间]`,
	`[팀]`,
	`[전체]`,
	`[All]`,
	`[Team]`,
	`[Party]`,
	`综合 한국어`,
}

func clipLog(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '	' || r >= 32 {
			return r
		}
		return -1
	}, s)
	if len([]rune(s)) > 80 {
		return string([]rune(s)[:80]) + "…"
	}
	return s
}

func debugLog(format string, args ...interface{}) {
	dlog.Printf(format, args...)
}

func (m *Monitor) AttachIfNeeded() error {
	p := m.proc()
	if p != nil && p.Alive() {
		return nil
	}
	pid, err := memory.FindProcess(memory.GameProcessName)
	if err != nil {
		return err
	}
	proc, err := memory.Open(pid)
	if err != nil {
		return err
	}
	m.mu.Lock()
	old := m.Proc
	m.Proc = proc
	m.buffers = nil
	m.mu.Unlock()
	if old != nil {
		old.Close()
	}
	debugLog("attached pid=%d admin=%v", pid, memory.IsAdmin())
	if _, title, werr := memory.FindGameWindow(pid); werr != nil {
		debugLog("game window: %v", werr)
	} else {
		debugLog("game window title=%q", title)
	}
	return nil
}

func (m *Monitor) CloseProc() {
	m.mu.Lock()
	p := m.Proc
	m.Proc = nil
	m.buffers = nil
	m.mu.Unlock()
	if p != nil {
		p.Close()
	}
}

func (m *Monitor) windowReady() bool {
	p := m.proc()
	if p == nil {
		return false
	}
	_, _, err := memory.FindGameWindow(p.PID)
	return err == nil
}

func (m *Monitor) pruneDead() int {
	p := m.proc()
	if p == nil {
		m.mu.Lock()
		m.buffers = nil
		m.mu.Unlock()
		return 0
	}
	m.mu.Lock()
	bufs := append([]buffer(nil), m.buffers...)
	m.mu.Unlock()

	alive := bufs[:0]
	for _, b := range bufs {
		if _, err := p.ReadMemory(b.addr, 8); err != nil {
			continue
		}
		alive = append(alive, b)
	}
	m.mu.Lock()
	m.buffers = alive
	n := len(m.buffers)
	m.mu.Unlock()
	return n
}

func (m *Monitor) LocatePassive(log func(string)) error {
	p := m.proc()
	if p == nil || !p.Alive() {
		return fmt.Errorf("进程未打开")
	}
	if !m.beginLocate() {
		return fmt.Errorf("正在初始化")
	}
	defer m.endLocate()
	return m.scanChatMarkers(log)
}

func (m *Monitor) scanChatMarkers(log func(string)) error {
	p := m.proc()
	if p == nil {
		return fmt.Errorf("进程未打开")
	}
	var patterns [][]byte
	var encs []string
	for _, mk := range chatMarkers {
		patterns = append(patterns, memory.EncodeProbe(mk, "utf-8"))
		encs = append(encs, "utf-8")
		patterns = append(patterns, memory.EncodeProbe(mk, "utf-16le"))
		encs = append(encs, "utf-16le")
	}
	hits, err := p.ScanPrivateRWMulti(patterns)
	if err != nil {
		debugLog("passive scan error: %v", err)
		return err
	}
	added := 0
	hitTotal := 0
	for i, addrs := range hits {
		hitTotal += len(addrs)
		if i >= len(encs) {
			break
		}
		enc := encs[i]
		for _, addr := range addrs {
			raw, rerr := p.ReadString(addr, 1024, enc)
			if rerr != nil || !memory.LooksLikeChat(raw) {
				continue
			}
			if m.addBuffer(enc, addr) {
				added++
				debugLog("passive keep enc=%s addr=%x raw=%q", enc, addr, clipLog(raw))
			}
		}
	}
	if m.BufferCount() == 0 {
		debugLog("passive locate: no chat markers patterns=%d rawHits=%d", len(patterns), hitTotal)
		return fmt.Errorf("未找到聊天控件")
	}
	if log != nil && added > 0 {
		log(fmt.Sprintf("定位 +%d", added))
	}
	debugLog("passive locate added=%d total=%d rawHits=%d", added, m.BufferCount(), hitTotal)
	return nil
}

func sleepStop(d time.Duration, stop <-chan struct{}) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-stop:
		return false
	case <-t.C:
		return true
	}
}

// AutoInit 挂上游戏后反复扫聊天记录（不是输入框前缀）。谁发的韩文都译。
func (m *Monitor) AutoInit(stop <-chan struct{}, sink Sink) {
	var lastErr string
	var lastCount int
	for {
		if err := m.AttachIfNeeded(); err != nil {
			if !sleepStop(3*time.Second, stop) {
				return
			}
			continue
		}
		m.pruneDead()
		if m.windowReady() {
			err := m.LocatePassive(nil)
			n := m.BufferCount()
			if err != nil {
				msg := err.Error()
				if msg != lastErr {
					debugLog("auto locate: %v", err)
					lastErr = msg
				}
			} else if n != lastCount {
				debugLog("auto locate buffers=%d", n)
			}
			lastCount = n
		}
		wait := 20 * time.Second
		if m.BufferCount() == 0 {
			wait = 3 * time.Second
		}
		if !sleepStop(wait, stop) {
			return
		}
	}
}
