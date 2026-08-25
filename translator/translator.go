package translator

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"hostrans/dlog"

	"github.com/andybalholm/brotli"
)

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

// Translator 接口
type Translator interface {
	Translate(text, from, to string) (string, error)
	Name() string
}

const (
	googleOrigin     = "https://translate.googleapis.com"
	deepLXDefaultURL = "https://trans.ors.de5.net/translate?token=1c6823aa2250"
	httpUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
)

// ==================== Google 公共接口（无 Key） ====================
type GoogleTranslator struct {
	client *http.Client
}

func NewGoogleTranslator() *GoogleTranslator {
	return &GoogleTranslator{client: &http.Client{Timeout: 8 * time.Second}}
}

func (g *GoogleTranslator) Name() string { return "Google" }

func googleLang(lang string) string {
	switch strings.ToLower(lang) {
	case "zh", "zh-cn", "zh-hans", "chinese":
		return "zh-CN"
	case "ko", "kor", "korean":
		return "ko"
	case "en", "eng", "english":
		return "en"
	case "auto", "":
		return "auto"
	default:
		return lang
	}
}

func (g *GoogleTranslator) Translate(text, from, to string) (string, error) {
	q := url.Values{}
	q.Set("client", "gtx")
	q.Set("sl", googleLang(from))
	q.Set("tl", googleLang(to))
	q.Set("dt", "t")
	q.Set("q", text)
	api := googleOrigin + "/translate_a/single?" + q.Encode()
	req, err := http.NewRequest("GET", api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", httpUserAgent)
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := rejectNonJSON(data); err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	var root []interface{}
	if err := json.Unmarshal(data, &root); err != nil || len(root) == 0 {
		return "", fmt.Errorf("解析失败")
	}
	outer, _ := root[0].([]interface{})
	var b strings.Builder
	for _, part := range outer {
		row, _ := part.([]interface{})
		if len(row) == 0 {
			continue
		}
		if s, ok := row[0].(string); ok {
			b.WriteString(s)
		}
	}
	out := b.String()
	if out == "" {
		return "", fmt.Errorf("empty")
	}
	return out, nil
}

// ==================== DeepLX ====================
type DeepLXTranslator struct {
	client   *http.Client
	endpoint string
}

func newDeepLXClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 12 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DisableCompression:    true,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
}

func NewDeepLXTranslator(endpoint string) *DeepLXTranslator {
	if endpoint == "" {
		endpoint = deepLXDefaultURL
	}
	return &DeepLXTranslator{
		client:   newDeepLXClient(),
		endpoint: endpoint,
	}
}

func (d *DeepLXTranslator) Name() string { return "DeepLX" }

func (d *DeepLXTranslator) setBrowserHeaders(req *http.Request) {
	origin := "https://trans.ors.de5.net"
	if u, err := url.Parse(d.endpoint); err == nil && u.Scheme != "" && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,ko;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("User-Agent", httpUserAgent)
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="151", "Chromium";v="151", "Not_A Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Priority", "u=1, i")
}

func looksJSON(b []byte) bool {
	s := bytes.TrimSpace(b)
	return len(s) > 0 && (s[0] == '{' || s[0] == '[')
}

func decodeHTTPBody(resp *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	isGzip := enc == "gzip" || (len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b)
	if isGzip {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		return io.ReadAll(io.LimitReader(gz, 1<<20))
	}
	if enc == "deflate" {
		fr := flate.NewReader(bytes.NewReader(raw))
		defer fr.Close()
		return io.ReadAll(io.LimitReader(fr, 1<<20))
	}
	if enc == "br" || enc == "brotli" || !looksJSON(raw) {
		out, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(raw)), 1<<20))
		if err == nil && looksJSON(out) {
			return out, nil
		}
		if enc == "br" || enc == "brotli" {
			if err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return raw, nil
}

func (d *DeepLXTranslator) postTranslate(body []byte) ([]byte, int, error) {
	req, err := http.NewRequest("POST", d.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	d.setBrowserHeaders(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := decodeHTTPBody(resp)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func parseDeepLX(data []byte) (string, error) {
	if err := rejectNonJSON(data); err != nil {
		return "", err
	}
	var result struct {
		Code    int    `json:"code"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("解析失败")
	}
	if result.Code != 200 {
		return "", fmt.Errorf("deeplx code %d: %s", result.Code, result.Message)
	}
	if result.Data == "" {
		return "", fmt.Errorf("empty")
	}
	return result.Data, nil
}

func (d *DeepLXTranslator) Translate(text, from, to string) (string, error) {
	if from == "" || from == "auto" {
		from = "auto"
	}
	payload := map[string]interface{}{
		"text":        text,
		"source_lang": strings.ToUpper(from),
		"target_lang": strings.ToUpper(to),
	}
	body, _ := json.Marshal(payload)

	data, status, err := d.postTranslate(body)
	if err != nil {
		return "", err
	}
	out, err := parseDeepLX(data)
	if err == nil {
		return out, nil
	}
	// cookie 罐第一次可能被拦，收 cookie 后再打一次（对应 curl -b/-c）
	if status == 403 || strings.Contains(err.Error(), "接口不可用") {
		data, _, err2 := d.postTranslate(body)
		if err2 != nil {
			return "", err
		}
		if out, err2 = parseDeepLX(data); err2 == nil {
			return out, nil
		}
		return "", err2
	}
	return "", err
}

// ==================== 多引擎自动回退 ====================
type Manager struct {
	engines []Translator
	mu      sync.Mutex
	cache   map[string]string
	order   []string
}

func NewManager() *Manager {
	return &Manager{
		engines: []Translator{
			NewGoogleTranslator(),
			NewDeepLXTranslator(""),
		},
		cache: make(map[string]string),
	}
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

// Translate 并发打所有引擎，谁先给出有效译文用谁（带短缓存）。
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

	type outcome struct {
		name string
		out  string
		err  error
	}
	ch := make(chan outcome, len(m.engines))
	for _, eng := range m.engines {
		eng := eng
		go func() {
			out, err := eng.Translate(text, from, to)
			if err == nil && !AcceptTranslation(text, out, to) {
				err = fmt.Errorf("%s 未译成目标语言", eng.Name())
			}
			ch <- outcome{name: eng.Name(), out: out, err: err}
		}()
	}

	var lastErr error
	for i := 0; i < len(m.engines); i++ {
		r := <-ch
		if r.err == nil && r.out != "" {
			m.remember(key, r.out)
			dlog.Debugf("trans %s %s→%s src=%q dst=%q", r.name, from, to, text, r.out)
			return r.out, nil
		}
		lastErr = r.err
		if r.out != "" {
			dlog.Infof("trans %s %s→%s REJECT src=%q dst=%q", r.name, from, to, text, r.out)
		} else {
			dlog.Infof("trans %s %s→%s FAIL src=%q err=%v", r.name, from, to, text, r.err)
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("翻译失败")
	}
	return "", fmt.Errorf("翻译失败")
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

func (m *Manager) Name() string {
	return "Multi-Engine"
}
