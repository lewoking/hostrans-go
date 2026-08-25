package translator

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"hostrans/dlog"
)

const httpUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

func rejectNonJSON(data []byte) error {
	s := bytes.TrimSpace(data)
	if len(s) == 0 {
		return fmt.Errorf("空响应")
	}
	if s[0] == '<' {
		return fmt.Errorf("接口不可用")
	}
	return nil
}

type Manager struct {
	ai    *AITranslator
	mu    sync.Mutex
	cache map[string]string
	order []string
}

func NewManager() *Manager {
	return &Manager{
		ai:    NewAITranslator(),
		cache: make(map[string]string),
	}
}

func (m *Manager) Warmup() {
	m.ai.Warmup()
}

func cacheKey(text, from, to string) string {
	return from + "\x00" + to + "\x00" + text
}

func clipRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

func (m *Manager) Translate(text, from, to string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	text = clipRunes(text, 240)
	key := cacheKey(text, from, to)
	m.mu.Lock()
	if v, ok := m.cache[key]; ok {
		m.mu.Unlock()
		return v, nil
	}
	m.mu.Unlock()

	out, err := m.ai.Translate(text, from, to)
	if err != nil {
		dlog.Infof("trans AI %s→%s FAIL src=%q err=%v", from, to, text, err)
		return "", fmt.Errorf("翻译失败")
	}
	if !AcceptTranslation(text, out, to) {
		dlog.Infof("trans AI %s→%s REJECT src=%q dst=%q", from, to, text, out)
		return "", fmt.Errorf("翻译失败")
	}
	m.remember(key, out)
	dlog.Debugf("trans AI %s→%s src=%q dst=%q", from, to, text, out)
	return out, nil
}

func (m *Manager) remember(key, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cache[key]; ok {
		return
	}
	if len(m.order) >= 400 {
		old := m.order[0]
		m.order = m.order[1:]
		delete(m.cache, old)
	}
	m.cache[key] = result
	m.order = append(m.order, key)
}
