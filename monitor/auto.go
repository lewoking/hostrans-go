package monitor

import (
	"fmt"
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
	if sink != nil {
		sink.Status("检测到对局，即将初始化")
		sink.Stay()
	}
	if !sleepStop(2*time.Second, stop) {
		return false
	}
	m.resetLock()

	for i := 0; i < 8; i++ {
		if err := m.AttachIfNeeded(); err != nil {
			if sink != nil {
				sink.Status("等待游戏进程…")
				sink.Stay()
			}
			if !sleepStop(2*time.Second, stop) {
				return false
			}
			continue
		}
		if err := m.LocatePassive(nil); err == nil && m.BufferCount() > 0 {
			if sink != nil {
				sink.Status(fmt.Sprintf("初始化完成，监听 %d 处", m.BufferCount()))
				sink.Show()
			}
			return true
		}
		// 加载画面往往还不能打字；多试几次被动扫描后再发探测串
		if i >= 3 && m.windowReady() {
			m.mu.Lock()
			tooSoon := !m.lastProbe.IsZero() && time.Since(m.lastProbe) < 45*time.Second
			m.mu.Unlock()
			if !tooSoon {
				if sink != nil {
					sink.Status("正在锁定当前界面聊天…")
					sink.Stay()
				}
				err := m.Locate(func(s string) {
					if sink != nil {
						sink.Status(s)
						sink.Stay()
					}
				})
				m.mu.Lock()
				m.lastProbe = time.Now()
				m.mu.Unlock()
				if err == nil && m.BufferCount() > 0 {
					if sink != nil {
						sink.Status("初始化完成，开始监控")
						sink.Show()
					}
					return true
				}
			}
		}
		if sink != nil {
			sink.Status("检测到对局，即将初始化…")
			sink.Stay()
		}
		if !sleepStop(2*time.Second, stop) {
			return false
		}
	}
	if sink != nil {
		sink.Status("自动初始化未成功，空聊天框按 Ctrl+P")
		sink.Stay()
	}
	return true
}

// AutoInit 借鉴 HotsStats：轮询 battlelobby 文件。
// 加载画面写出 replay.server.battlelobby 即视为新对局，提示后自动初始化。
func (m *Monitor) AutoInit(stop <-chan struct{}, sink Sink) {
	var lastMod time.Time
	for {
		_ = m.AttachIfNeeded()
		m.pruneDead()

		mt, ok := battleLobbyModTime()
		if !ok {
			lastMod = time.Time{}
		} else if !mt.Equal(lastMod) {
			lastMod = mt
			if !m.initAfterLobby(stop, sink) {
				return
			}
			continue
		}

		if m.BufferCount() == 0 {
			if sink != nil {
				if ok {
					sink.Status("对局进行中，空聊天框按 Ctrl+P 初始化")
				} else {
					sink.Status("等待进入对局…")
				}
				sink.Stay()
			}
		}

		if !sleepStop(time.Second, stop) {
			return
		}
	}
}
