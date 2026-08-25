package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hostrans/memory"
)

var chatMarkers = []string{
	`<c val="3184FF">[团队]:</c>`,
	`<c val="3184FF">[队伍]:</c>`,
	`[团队]:`,
	`[队伍]:`,
	`[组队]:`,
	`[팀]`,
	`[전체]`,
	`[All]`,
	`[Team]`,
	`[Party]`,
	`综合 한국어`,
}

func debugLog(format string, args ...interface{}) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "hostrans.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, time.Now().Format("15:04:05 ")+format+"\n", args...)
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
	debugLog("attached pid=%d", pid)
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
		return err
	}
	added := 0
	for i, addrs := range hits {
		if i >= len(encs) {
			break
		}
		enc := encs[i]
		for _, addr := range addrs {
			if m.addBuffer(enc, addr) {
				added++
			}
		}
	}
	if m.BufferCount() == 0 {
		return fmt.Errorf("未找到聊天控件")
	}
	if log != nil && added > 0 {
		log(fmt.Sprintf("定位 +%d", added))
	}
	debugLog("passive locate added=%d total=%d", added, m.BufferCount())
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

// AutoInit 只等游戏进程；初始化改由空聊天框 Ctrl+P 触发。
func (m *Monitor) AutoInit(stop <-chan struct{}, sink Sink) {
	for {
		if err := m.AttachIfNeeded(); err != nil {
			if sink != nil {
				sink.Status("等待游戏")
			}
			if !sleepStop(3*time.Second, stop) {
				return
			}
			continue
		}
		m.pruneDead()
		if m.BufferCount() == 0 && sink != nil {
			sink.Status("空框 Ctrl+P 初始化")
		}
		if !sleepStop(3*time.Second, stop) {
			return
		}
	}
}
