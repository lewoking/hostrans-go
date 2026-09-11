package monitor

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"hostrans/dlog"
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
	addr   uintptr
	enc    string
	last   string
	fail   int
	hangul bool
	draft  bool
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

func (s *seenSet) Has(x string) bool {
	_, ok := s.m[x]
	return ok
}

func (s *seenSet) CoveredBy(x string) bool {
	if x == "" {
		return false
	}
	for _, old := range s.q {
		if len(old) > len(x) && strings.Contains(old, x) {
			return true
		}
	}
	return false
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
		jobs:   make(chan translateJob, 32),
	}
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
	return m.insertBuffer(buffer{addr: addr, enc: enc, hangul: true})
}

func (m *Monitor) insertBuffer(nb buffer) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range m.buffers {
		if b.addr == nb.addr && b.enc == nb.enc {
			if nb.hangul {
				m.buffers[i].hangul = true
			}
			if nb.draft {
				m.buffers[i].draft = true
			}
			return false
		}
	}
	if len(m.buffers) >= 32 {
		drop := 0
		for i, b := range m.buffers {
			if !b.hangul && !b.draft {
				drop = i
				break
			}
		}
		if m.buffers[drop].draft || m.buffers[drop].hangul {
			for i, b := range m.buffers {
				if !b.draft {
					drop = i
					break
				}
			}
		}
		m.buffers = append(m.buffers[:drop], m.buffers[drop+1:]...)
	}
	m.buffers = append(m.buffers, nb)
	return true
}

func (m *Monitor) proc() *memory.Process {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Proc
}

// Locate 先被动扫聊天标记；找不到再发探测串。
// 聊天记录是追加写入：同一地址不会被下一条覆盖，因此每轮全量重扫，
// 有交集用交集（原地缓冲），没有则用本轮命中（追加型）。
func (m *Monitor) Locate(log func(string)) error {
	p := m.proc()
	if p == nil || !p.Alive() {
		return fmt.Errorf("进程未打开")
	}
	if !m.beginLocate() {
		return fmt.Errorf("正在初始化")
	}
	defer m.endLocate()

	if err := m.scanChatMarkers(log); err == nil && m.BufferCount() > 0 {
		debugLog("locate: passive ok total=%d", m.BufferCount())
		return nil
	}

	var utf8Hits, utf16Hits []uintptr
	for i := 0; i < 3; i++ {
		probe := memory.RandomProbe()
		m.mu.Lock()
		m.probes[probe] = struct{}{}
		m.mu.Unlock()
		if log != nil {
			log(fmt.Sprintf("探测 %d/3 %s", i+1, probe))
		}
		debugLog("locate: send probe %d/3 %s", i+1, probe)
		if err := memory.SendChat(p.PID, probe); err != nil {
			debugLog("locate: SendChat: %v", err)
			return err
		}
		p8 := memory.EncodeProbe(probe, "utf-8")
		p16 := memory.EncodeProbe(probe, "utf-16le")
		u8, err := p.ScanPrivateRW(p8)
		if err != nil {
			debugLog("locate: utf8 scan: %v", err)
			return err
		}
		u16, err := p.ScanPrivateRW(p16)
		if err != nil {
			debugLog("locate: utf16 scan: %v", err)
			return err
		}
		if i == 0 {
			utf8Hits, utf16Hits = u8, u16
		} else {
			utf8Hits = MergeProbeHits(utf8Hits, u8)
			utf16Hits = MergeProbeHits(utf16Hits, u16)
		}
		debugLog("locate: probe %s utf8=%d utf16=%d keep8=%d keep16=%d",
			probe, len(u8), len(u16), len(utf8Hits), len(utf16Hits))
		if log != nil {
			log(fmt.Sprintf("命中 %d/%d", len(utf8Hits), len(utf16Hits)))
		}
		if len(utf8Hits) == 0 && len(utf16Hits) == 0 {
			return fmt.Errorf("探测失败")
		}
	}
	added := 0
	for _, addr := range utf8Hits {
		if m.addBuffer("utf-8", addr) {
			added++
		}
	}
	for _, addr := range utf16Hits {
		if m.addBuffer("utf-16le", addr) {
			added++
		}
	}
	if m.BufferCount() == 0 {
		return fmt.Errorf("初始化失败")
	}
	if log != nil {
		log(fmt.Sprintf("已监听 %d 处", m.BufferCount()))
	}
	debugLog("locate: added=%d total=%d", added, m.BufferCount())
	return nil
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
		raw, err := p.ReadString(bufs[i].addr, 1024, bufs[i].enc)
		if err != nil {
			bufs[i].fail++
			changed = true
			continue
		}
		bufs[i].fail = 0
		// 多读一段，同一页里新冒出来的韩文也能抓到，不必等下一轮全堆扫描
		if win, werr := p.ReadMemory(bufs[i].addr, 2048); werr == nil && len(win) > 0 {
			chunk := win
			const back = 256
			if bufs[i].addr > back {
				if pre, perr := p.ReadMemory(bufs[i].addr-back, back); perr == nil && len(pre) > 0 {
					chunk = append(pre, win...)
				}
			}
			raw = memory.WindowToString(chunk, bufs[i].enc)
		}
		if raw == "" || raw == bufs[i].last {
			continue
		}
		bufs[i].last = raw
		if memory.LooksLikeChat(raw) || memory.ContainsKorean(raw) {
			bufs[i].hangul = true
		}
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
			b.hangul = b.hangul || u.hangul
		}
		if b.fail < 3 {
			alive = append(alive, b)
		}
	}
	m.buffers = alive
}

func (m *Monitor) emit(raw, lastMine string, probes map[string]struct{}, sink Sink) {
	for _, line := range memory.ChatCandidates(raw) {
		m.emitLine(line, lastMine, probes, sink)
	}
}

func (m *Monitor) emitLine(line memory.ChatLine, lastMine string, probes map[string]struct{}, sink Sink) {
	if memory.ShouldSkip(line, probes) {
		return
	}
	body := line.Text
	if body == "" {
		return
	}
	if lastMine != "" && body == lastMine {
		return
	}
	if m.seen.Has(body) || m.seen.CoveredBy(body) {
		return
	}
	if m.seen.Add(body) {
		return
	}
	who := memory.DisplayWho(line)
	dlog.Infof("chat who=%q body=%q", who, body)
	if !memory.NeedsTranslate(body) {
		debugLog("queue chat speaker=%q body=%q", who, body)
		if sink != nil {
			sink.Push(who, body)
			sink.Show()
		}
		return
	}
	debugLog("queue ko speaker=%q body=%q", who, body)
	job := translateJob{speaker: who, body: body}
	select {
	case m.jobs <- job:
	default:
	}
}

// RunTranslator 多工人并行译不同句子。同一句仍是引擎赛跑。
func (m *Monitor) RunTranslator(stop <-chan struct{}, sink Sink) {
	m.RunTranslators(4, stop, sink)
}

func (m *Monitor) RunTranslators(n int, stop <-chan struct{}, sink Sink) {
	if n < 1 {
		n = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				case job := <-m.jobs:
					zh, err := m.Trans.Translate(job.body, "auto", "zh")
					if sink == nil {
						continue
					}
					if err != nil || zh == "" || looksLikeFailure(zh) {
						debugLog("ko→zh fail speaker=%q body=%q err=%v dst=%q", job.speaker, job.body, err, zh)
						dlog.Errorf("翻译失败")
						if strings.ContainsAny(job.body, "<>%") || strings.Contains(job.body, "storm_ui_") || strings.Contains(job.body, ".dds") {
							continue
						}
						sink.Push(job.speaker, job.body)
						sink.Show()
						continue
					}
					sink.Push(job.speaker, zh)
					sink.Show()
				}
			}
		}()
	}
	wg.Wait()
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

func (m *Monitor) TranslateInput(sink Sink) {
	p := m.proc()
	if p == nil {
		dlog.Errorf("未找到游戏")
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
		src = ""
	}
	src = trimChat(src)
	kind := memory.ClassifyInput(src)
	debugLog("input capture src=%q kind=%v err=%v", src, kind, err)
	switch kind {
	case memory.InputEmpty, memory.InputOther:
		if err := m.Locate(nil); err != nil {
			debugLog("manual locate: %v", err)
			dlog.Errorf("初始化失败")
		}
		return
	case memory.InputKorean:
		zh, err := m.Trans.Translate(src, "ko", "zh")
		if err != nil || zh == "" || looksLikeFailure(zh) {
			debugLog("ko→zh input fail src=%q err=%v dst=%q", src, err, zh)
			dlog.Errorf("韩译中失败")
			return
		}
		debugLog("ko→zh input src=%q dst=%q", src, zh)
		if sink != nil {
			sink.Push("我", zh)
			sink.Show()
		}
		return
	case memory.InputChinese:
		dst, err := m.Trans.Translate(src, "zh", "en")
		if err != nil || dst == "" || looksLikeFailure(dst) {
			debugLog("zh→en fail src=%q err=%v dst=%q", src, err, dst)
			dlog.Errorf("中译英失败")
			return
		}
		debugLog("zh→en src=%q dst=%q", src, dst)
		if err := memory.TranslateChatBox(p.PID, dst); err != nil {
			debugLog("fill-back fail: %v", err)
			dlog.Errorf("发送失败")
			return
		}
		m.mu.Lock()
		m.lastMine = dst
		m.mu.Unlock()
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
