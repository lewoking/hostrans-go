package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hostrans/memory"
)

// 选人界面、局内 HUD 常见的聊天占位/频道标记。
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

// LocatePassive 不发聊天，靠频道标记找出选人/局内控件。
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
		log(fmt.Sprintf("被动定位 +%d，当前监听 %d 处", added, m.BufferCount()))
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

func (m *Monitor) initAfterLobby(stop <-chan struct{}, sink Sink) bool {
	debugLog("battlelobby → init")
	if sink != nil {
		sink.Status("检测到对局，即将初始化")
		sink.Stay()
	}
	// 等加载画面稍稳，期间不要反复扫全堆
	if !sleepStop(5*time.Second, stop) {
		return false
	}
	m.resetLock()
	_ = m.AttachIfNeeded()

	if err := m.LocatePassive(nil); err == nil && m.BufferCount() > 0 {
		if sink != nil {
			sink.Status(fmt.Sprintf("初始化完成，监听 %d 处", m.BufferCount()))
			sink.Show()
		}
		return true
	}
	if !sleepStop(5*time.Second, stop) {
		return false
	}
	if err := m.LocatePassive(nil); err == nil && m.BufferCount() > 0 {
		if sink != nil {
			sink.Status(fmt.Sprintf("初始化完成，监听 %d 处", m.BufferCount()))
			sink.Show()
		}
		return true
	}

	// 不自动发探测串（会抢键鼠、误发聊天）。空框 Ctrl+P 才探测。
	if sink != nil {
		sink.Status("检测到对局。打开空聊天框按 Ctrl+P 初始化")
		sink.Show()
	}
	debugLog("auto init: passive miss, wait Ctrl+P")
	return true
}

// AutoInit 借鉴 HotsStats：轮询 battlelobby。
// 只在「新写出/mtime 变化」时触发，忽略启动时已经存在的旧文件。
func (m *Monitor) AutoInit(stop <-chan struct{}, sink Sink) {
	var lastMod time.Time
	watching := false
	for {
		_ = m.AttachIfNeeded()
		m.pruneDead()

		mt, ok := battleLobbyModTime()
		if !ok {
			watching = false
			lastMod = time.Time{}
		} else if !watching {
			watching = true
			lastMod = mt
			// 启动时就在加载画面（文件很新）才立刻跟；大厅残留的旧文件忽略
			if isFreshLobby(mt, 90*time.Second) {
				if !m.initAfterLobby(stop, sink) {
					return
				}
			} else {
				debugLog("ignore stale battlelobby age=%s", time.Since(mt))
			}
		} else if !mt.Equal(lastMod) {
			lastMod = mt
			if !m.initAfterLobby(stop, sink) {
				return
			}
		}

		if !sleepStop(time.Second, stop) {
			return
		}
	}
}
