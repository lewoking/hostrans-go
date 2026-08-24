package monitor

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"hostrans/memory"
	"hostrans/translator"
)

type Sink interface {
	Push(speaker, text string)
	Status(msg string)
	Show()
}

type Monitor struct {
	Proc  *memory.Process
	Trans *translator.Manager

	mu       sync.Mutex
	addr     uintptr
	encoding string
	ready    bool
	probes   map[string]struct{}
	lastMine string
	lastRaw  string
	seen     *seenSet
}

type seenSet struct {
	m map[string]struct{}
	q []string
}

func newSeen() *seenSet {
	return &seenSet{m: make(map[string]struct{})}
}

func (s *seenSet) Add(x string) bool {
	if x == "" {
		return true
	}
	if _, ok := s.m[x]; ok {
		return true
	}
	s.m[x] = struct{}{}
	s.q = append(s.q, x)
	if len(s.q) > 80 {
		old := s.q[0]
		s.q = s.q[1:]
		delete(s.m, old)
	}
	return false
}

func New(proc *memory.Process, trans *translator.Manager) *Monitor {
	return &Monitor{
		Proc:   proc,
		Trans:  trans,
		probes: make(map[string]struct{}),
		seen:   newSeen(),
	}
}

func (m *Monitor) Ready() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

func (m *Monitor) AddrHex() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fmt.Sprintf("%s@0x%X", m.encoding, m.addr)
}

// Locate 发送 3 次唯一探测串，同时用 utf-8 / utf-16le 求交，锁定聊天缓冲区。
func (m *Monitor) Locate(log func(string)) error {
	if m.Proc == nil {
		return fmt.Errorf("进程未打开")
	}
	var utf8Hits, utf16Hits []uintptr
	for i := 0; i < 3; i++ {
		probe := memory.RandomProbe()
		m.mu.Lock()
		m.probes[probe] = struct{}{}
		m.mu.Unlock()
		if log != nil {
			log(fmt.Sprintf("发送探测 %d/3: %s", i+1, probe))
		}
		if err := memory.SendChat(m.Proc.PID, probe); err != nil {
			return err
		}
		p8 := memory.EncodeProbe(probe, "utf-8")
		p16 := memory.EncodeProbe(probe, "utf-16le")
		if i == 0 {
			var err error
			utf8Hits, err = m.Proc.ScanPrivateRW(p8)
			if err != nil {
				return err
			}
			utf16Hits, err = m.Proc.ScanPrivateRW(p16)
			if err != nil {
				return err
			}
		} else {
			utf8Hits = m.Proc.FilterContains(utf8Hits, p8)
			utf16Hits = m.Proc.FilterContains(utf16Hits, p16)
		}
		if log != nil {
			log(fmt.Sprintf("候选 utf-8=%d  utf-16le=%d", len(utf8Hits), len(utf16Hits)))
		}
		if len(utf8Hits) == 0 && len(utf16Hits) == 0 {
			return fmt.Errorf("未扫到探测串，请确认已进入对局且窗口在前台")
		}
	}
	enc, addr, err := pickAddr(utf8Hits, utf16Hits)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.addr = addr
	m.encoding = enc
	m.ready = true
	m.mu.Unlock()
	if log != nil {
		log(fmt.Sprintf("初始化成功，监听 %s@0x%X", enc, addr))
	}
	return nil
}

func pickAddr(utf8Hits, utf16Hits []uintptr) (string, uintptr, error) {
	type cand struct {
		enc   string
		addrs []uintptr
	}
	opts := []cand{{"utf-8", utf8Hits}, {"utf-16le", utf16Hits}}
	for _, o := range opts {
		if len(o.addrs) == 1 {
			return o.enc, o.addrs[0], nil
		}
	}
	for _, o := range opts {
		if len(o.addrs) > 1 && len(o.addrs) <= 5 {
			return o.enc, o.addrs[0], nil
		}
	}
	return "", 0, fmt.Errorf("初始化失败：请进对局后重试，并改用窗口化最大化")
}

func (m *Monitor) Tick(sink Sink) {
	if m.Proc == nil || !m.Proc.Alive() {
		if sink != nil {
			sink.Status("游戏未启动")
			sink.Show()
		}
		return
	}
	m.mu.Lock()
	ready := m.ready
	addr := m.addr
	enc := m.encoding
	lastMine := m.lastMine
	probes := make(map[string]struct{}, len(m.probes))
	for k, v := range m.probes {
		probes[k] = v
	}
	m.mu.Unlock()
	if !ready {
		return
	}

	raw, err := m.Proc.ReadString(addr, 320, enc)
	if err != nil || raw == "" {
		return
	}
	m.mu.Lock()
	if raw == m.lastRaw {
		m.mu.Unlock()
		return
	}
	m.lastRaw = raw
	m.mu.Unlock()

	line := memory.ParseChatLine(raw)
	if memory.ShouldSkip(line, probes) {
		return
	}
	body := line.Text
	if body == "" {
		body = memory.ParseChatLine(raw).Raw
	}
	if lastMine != "" && (body == lastMine || raw == lastMine) {
		return
	}
	if m.seen.Add(raw) {
		return
	}
	if !memory.NeedsTranslate(body) {
		return
	}

	zh, err := m.Trans.Translate(body, "auto", "zh")
	if err != nil || zh == "" {
		return
	}
	if looksLikeFailure(zh) {
		return
	}
	speaker := memory.DisplaySpeaker(line)
	if sink != nil {
		sink.Push(speaker, zh)
		sink.Show()
	}
}

func looksLikeFailure(s string) bool {
	return s == "翻译失败!" || s == "翻译失败"
}

func (m *Monitor) Loop(d time.Duration, sink Sink, stop <-chan struct{}) {
	t := time.NewTicker(d)
	defer t.Stop()
	var busy int32
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if !atomic.CompareAndSwapInt32(&busy, 0, 1) {
				continue
			}
			m.Tick(sink)
			atomic.StoreInt32(&busy, 0)
		}
	}
}

// TranslateInput Ctrl+P：把输入框中文译成韩语并粘贴回去。
func (m *Monitor) TranslateInput(log func(string)) {
	src, err := memory.CaptureChatInput(m.Proc.PID)
	if err != nil {
		if log != nil {
			log("读取输入框失败: " + err.Error())
		}
		return
	}
	src = trimChat(src)
	if src == "" {
		return
	}
	ko, err := m.Trans.Translate(src, "zh", "ko")
	if err != nil {
		if log != nil {
			log("翻译失败: " + err.Error())
		}
		return
	}
	if err := memory.PasteToGame(m.Proc.PID, ko); err != nil {
		if log != nil {
			log("粘贴失败: " + err.Error())
		}
		return
	}
	m.mu.Lock()
	m.lastMine = ko
	m.mu.Unlock()
	if log != nil {
		log("已译韩: " + ko)
	}
}

func trimChat(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == 0 || !unicode.IsControl(r) || r == '\n' || r == '\t' {
			if r != 0 {
				out = append(out, r)
			}
		}
	}
	return string(out)
}
