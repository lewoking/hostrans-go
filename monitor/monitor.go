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
	Replace(speaker, from, to string)
	Status(msg string)
	Show()
	Stay()
}

type buffer struct {
	addr   uintptr
	enc    string
	last   string
	known  map[string]struct{}
	primed bool
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

	mu            sync.Mutex
	Proc          *memory.Process
	buffers       []buffer
	probes        map[string]struct{}
	lastMine      string
	lastProbe     time.Time
	seen          *seenSet
	locating      int32
	baselineReady bool
	jobs          chan translateJob
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
	if len(s.q) > 400 {
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
	// 命中的是渲染缓存中的字符串地址，而不是带时间戳或消息 ID 的聊天对象。
	// 新地址需要在 Tick 中结合全局历史正文集合判断，不能仅凭地址决定新旧。
	nb.primed = false
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
	baselineReady := m.baselineReady
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
		// addr 是聊天字符串内的命中位置，不是聊天结构的起始地址。
		// ReadString 已按该字符串编码读到 NUL 终止符；不得拼接邻近游戏内存，
		// 否则会把 UI、旧消息或对象字段误作为聊天正文发给翻译服务。
		if !bufs[i].primed {
			// 聊天字符串没有时间戳或消息 ID。首轮扫描只把所有正文加入
			// 本次 EXE 运行期的内存缓存，不上窗；之后新地址里的未知正文
			// 才作为实时消息立即处理。
			if baselineReady {
				m.emitNew(raw, nil, lastMine, probes, sink)
			} else {
				for _, line := range memory.ChatCandidates(raw) {
					if line.Text != "" {
						m.seen.Add(line.Text)
					}
				}
			}
			bufs[i].last = raw
			bufs[i].known = snapshotBodies(raw)
			bufs[i].primed = true
			changed = true
			continue
		}
		if raw == "" || raw == bufs[i].last {
			continue
		}
		bufs[i].last = raw
		if memory.LooksLikeChat(raw) || memory.ContainsKorean(raw) {
			bufs[i].hangul = true
		}
		changed = true
		m.emitNew(raw, bufs[i].known, lastMine, probes, sink)
		bufs[i].known = snapshotBodies(raw)
	}

	if !changed {
		m.mu.Lock()
		if !m.baselineReady && len(bufs) > 0 {
			m.baselineReady = true
		}
		m.mu.Unlock()
		return
	}
	m.mu.Lock()
	if !m.baselineReady {
		// seen 中保存启动时可见的历史正文；完全只在内存中，EXE 退出即清空。
		m.baselineReady = true
	}
	defer m.mu.Unlock()
	byKey := make(map[string]buffer, len(bufs))
	for _, b := range bufs {
		byKey[fmt.Sprintf("%s:%x", b.enc, b.addr)] = b
	}
	alive := m.buffers[:0]
	for _, b := range m.buffers {
		if u, ok := byKey[fmt.Sprintf("%s:%x", b.enc, b.addr)]; ok {
			b.last, b.fail, b.primed = u.last, u.fail, u.primed
			b.hangul = b.hangul || u.hangul
			if u.known != nil {
				b.known = u.known
			}
		}
		if b.fail < 3 {
			alive = append(alive, b)
		}
	}
	m.buffers = alive
}

func snapshotBodies(raw string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, line := range memory.ChatCandidates(raw) {
		if line.Text != "" {
			out[line.Text] = struct{}{}
		}
	}
	return out
}

func (m *Monitor) emitNew(raw string, known map[string]struct{}, lastMine string, probes map[string]struct{}, sink Sink) {
	for _, line := range memory.ChatCandidates(raw) {
		if line.Text == "" {
			continue
		}
		if _, ok := known[line.Text]; ok {
			continue
		}
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
	if sink != nil {
		sink.Push(who, body)
		sink.Show()
	}
	if !memory.NeedsTranslate(body) {
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
						continue
					}
					sink.Replace(job.speaker, job.body, zh)
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
	dlog.Infof("input capture src=%q kind=%v err=%v", src, kind, err)
	switch kind {
	case memory.InputEmpty, memory.InputOther:
		if err := m.Locate(nil); err != nil {
			dlog.Infof("manual locate: %v", err)
			debugLog("manual locate: %v", err)
			dlog.Errorf("初始化失败")
		}
		return
	case memory.InputKorean:
		zh, err := m.Trans.Translate(src, "ko", "zh")
		if err != nil || zh == "" || looksLikeFailure(zh) {
			dlog.Infof("ko→zh input fail src=%q err=%v dst=%q", src, err, zh)
			debugLog("ko→zh input fail src=%q err=%v dst=%q", src, err, zh)
			dlog.Errorf("韩译中失败")
			return
		}
		dlog.Infof("ko→zh input src=%q dst=%q", src, zh)
		debugLog("ko→zh input src=%q dst=%q", src, zh)
		if sink != nil {
			sink.Push("我", zh)
			sink.Show()
		}
		return
	case memory.InputChinese:
		dst, err := m.Trans.Translate(src, "zh", "en")
		if err != nil || dst == "" || looksLikeFailure(dst) {
			dlog.Infof("zh→en fail src=%q err=%v dst=%q", src, err, dst)
			debugLog("zh→en fail src=%q err=%v dst=%q", src, err, dst)
			dlog.Errorf("中译英失败")
			return
		}
		dlog.Infof("zh→en src=%q dst=%q", src, dst)
		debugLog("zh→en src=%q dst=%q", src, dst)
		if err := memory.TranslateChatBox(p.PID, dst); err != nil {
			dlog.Infof("fill-back fail: %v", err)
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
