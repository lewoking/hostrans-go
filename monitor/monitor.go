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
	Stay()
}

type buffer struct {
	addr uintptr
	enc  string
	last string
	fail int
}

type translateJob struct {
	speaker string
	body    string
}

type Monitor struct {
	Trans *translator.Manager

	mu        sync.Mutex
	Proc      *memory.Process
	buffers   []buffer
	probes    map[string]struct{}
	lastMine  string
	lastProbe time.Time
	seen      *seenSet
	locating  int32
	jobs      chan translateJob
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
	if len(s.q) > 120 {
		old := s.q[0]
		s.q = s.q[1:]
		delete(s.m, old)
	}
	return false
}

func New(trans *translator.Manager) *Monitor {
	return &Monitor{
		Trans:  trans,
		probes: make(map[string]struct{}),
		seen:   newSeen(),
		jobs:   make(chan translateJob, 24),
	}
}

func (m *Monitor) Ready() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.buffers) > 0
}

func (m *Monitor) BufferCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.buffers)
}

func (m *Monitor) beginLocate() bool {
	return atomic.CompareAndSwapInt32(&m.locating, 0, 1)
}

func (m *Monitor) endLocate() {
	atomic.StoreInt32(&m.locating, 0)
}

func (m *Monitor) addBuffer(enc string, addr uintptr) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.buffers {
		if b.addr == addr && b.enc == enc {
			return false
		}
	}
	if len(m.buffers) >= 16 {
		m.buffers = m.buffers[1:]
	}
	m.buffers = append(m.buffers, buffer{addr: addr, enc: enc})
	return true
}

func (m *Monitor) proc() *memory.Process {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Proc
}

// Locate 向当前场景聊天框发探测串并加入监听列表。
// 选人、局内各做一次即可两边都覆盖；已有缓冲时仍会找当前场景的新地址。
func (m *Monitor) Locate(log func(string)) error {
	p := m.proc()
	if p == nil || !p.Alive() {
		return fmt.Errorf("进程未打开")
	}
	if !m.beginLocate() {
		return fmt.Errorf("正在初始化")
	}
	defer m.endLocate()

	var utf8Hits, utf16Hits []uintptr
	for i := 0; i < 3; i++ {
		probe := memory.RandomProbe()
		m.mu.Lock()
		m.probes[probe] = struct{}{}
		m.mu.Unlock()
		if log != nil {
			log(fmt.Sprintf("发送探测 %d/3: %s", i+1, probe))
		}
		if err := memory.SendChat(p.PID, probe); err != nil {
			return err
		}
		p8 := memory.EncodeProbe(probe, "utf-8")
		p16 := memory.EncodeProbe(probe, "utf-16le")
		if i == 0 {
			var err error
			utf8Hits, err = p.ScanPrivateRW(p8)
			if err != nil {
				return err
			}
			utf16Hits, err = p.ScanPrivateRW(p16)
			if err != nil {
				return err
			}
		} else {
			utf8Hits = p.FilterContains(utf8Hits, p8)
			utf16Hits = p.FilterContains(utf16Hits, p16)
		}
		if log != nil {
			log(fmt.Sprintf("候选 utf-8=%d  utf-16le=%d", len(utf8Hits), len(utf16Hits)))
		}
		if len(utf8Hits) == 0 && len(utf16Hits) == 0 {
			return fmt.Errorf("未扫到探测串，请确认当前界面能打字")
		}
	}
	enc, addr, err := pickAddr(utf8Hits, utf16Hits)
	if err != nil {
		return err
	}
	added := m.addBuffer(enc, addr)
	if log != nil {
		if added {
			log(fmt.Sprintf("已监听 %s@0x%X（当前共 %d 处，含选人/局内）", enc, addr, m.BufferCount()))
		} else {
			log("当前场景已在监听")
		}
	}
	return nil
}

func pickAddr(utf8Hits, utf16Hits []uintptr) (string, uintptr, error) {
	type hit struct {
		enc   string
		addrs []uintptr
	}
	opts := []hit{{"utf-8", utf8Hits}, {"utf-16le", utf16Hits}}
	for _, o := range opts {
		if len(o.addrs) == 1 {
			return o.enc, o.addrs[0], nil
		}
	}
	for _, o := range opts {
		if len(o.addrs) > 1 && len(o.addrs) <= 8 {
			return o.enc, o.addrs[0], nil
		}
	}
	return "", 0, fmt.Errorf("初始化失败：请在能打字的界面重试（选人或局内）")
}

func (m *Monitor) Tick(sink Sink) {
	p := m.proc()
	if p == nil || !p.Alive() {
		m.mu.Lock()
		m.buffers = nil
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	bufs := append([]buffer(nil), m.buffers...)
	lastMine := m.lastMine
	probes := make(map[string]struct{}, len(m.probes))
	for k, v := range m.probes {
		probes[k] = v
	}
	m.mu.Unlock()
	if len(bufs) == 0 {
		return
	}

	changed := false
	for i := range bufs {
		raw, err := p.ReadString(bufs[i].addr, 320, bufs[i].enc)
		if err != nil {
			bufs[i].fail++
			changed = true
			continue
		}
		bufs[i].fail = 0
		if raw == "" || raw == bufs[i].last {
			continue
		}
		bufs[i].last = raw
		changed = true
		m.emit(raw, lastMine, probes, sink)
	}

	if !changed {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	byKey := make(map[string]buffer, len(bufs))
	for _, b := range bufs {
		byKey[fmt.Sprintf("%s:%x", b.enc, b.addr)] = b
	}
	alive := m.buffers[:0]
	for _, b := range m.buffers {
		if u, ok := byKey[fmt.Sprintf("%s:%x", b.enc, b.addr)]; ok {
			b.last, b.fail = u.last, u.fail
		}
		if b.fail < 3 {
			alive = append(alive, b)
		}
	}
	m.buffers = alive
}

func (m *Monitor) emit(raw, lastMine string, probes map[string]struct{}, sink Sink) {
	line := memory.ParseChatLine(raw)
	if memory.ShouldSkip(line, probes) {
		return
	}
	body := line.Text
	if body == "" {
		return
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
	job := translateJob{speaker: memory.DisplaySpeaker(line), body: body}
	select {
	case m.jobs <- job:
	default:
	}
}

// RunTranslator 独立协程翻译，避免 HTTP 堵住 0.8s 内存轮询。
func (m *Monitor) RunTranslator(stop <-chan struct{}, sink Sink) {
	for {
		select {
		case <-stop:
			return
		case job := <-m.jobs:
			zh, err := m.Trans.Translate(job.body, "auto", "zh")
			if err != nil || zh == "" || looksLikeFailure(zh) {
				continue
			}
			if sink != nil {
				sink.Push(job.speaker, zh)
				sink.Show()
			}
		}
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

func (m *Monitor) TranslateInput(log func(string)) {
	p := m.proc()
	if p == nil {
		if log != nil {
			log("游戏未连接")
		}
		return
	}
	oldClip, clipErr := memory.GetClipboardText()
	defer func() {
		if clipErr == nil {
			_ = memory.SetClipboardText(oldClip)
		}
	}()
	src, err := memory.CaptureChatInput(p.PID)
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
	dst, err := m.Trans.Translate(src, "zh", "ko")
	if err != nil {
		if log != nil {
			log("翻译失败: " + err.Error())
		}
		return
	}
	if err := memory.TranslateChatBox(p.PID, dst); err != nil {
		if log != nil {
			log("粘贴失败: " + err.Error())
		}
		return
	}
	m.mu.Lock()
	m.lastMine = dst
	m.mu.Unlock()
	if log != nil {
		log("已译韩: " + dst)
	}
}

func trimChat(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r != 0 && (!unicode.IsControl(r) || r == '\n' || r == '\t') {
			out = append(out, r)
		}
	}
	return string(out)
}
