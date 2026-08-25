package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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

// ==================== 微软免费翻译（无 Key） ====================
type MicrosoftTranslator struct {
	client *http.Client
}

func NewMicrosoftTranslator() *MicrosoftTranslator {
	return &MicrosoftTranslator{
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (m *MicrosoftTranslator) Name() string { return "Microsoft" }

var (
	edgeMu    sync.Mutex
	edgeToken string
	edgeUntil time.Time
)

func fetchEdgeToken(client *http.Client) string {
	edgeMu.Lock()
	defer edgeMu.Unlock()
	if edgeToken != "" && time.Now().Before(edgeUntil) {
		return edgeToken
	}
	req, err := http.NewRequest("GET", "https://edge.microsoft.com/translate/auth", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	tok := strings.TrimSpace(string(data))
	if tok == "" || resp.StatusCode != 200 {
		return ""
	}
	edgeToken = tok
	edgeUntil = time.Now().Add(8 * time.Minute)
	return tok
}

func (m *MicrosoftTranslator) Translate(text, from, to string) (string, error) {
	apiURL := "https://api-edge.cognitive.microsofttranslator.com/translate?api-version=3.0&to=" + url.QueryEscape(to)
	if from != "" && from != "auto" {
		apiURL += "&from=" + url.QueryEscape(from)
	}

	payload, _ := json.Marshal([]map[string]string{{"Text": text}})
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Edg/120.0.0.0")
	tok := fetchEdgeToken(m.client)
	if tok == "" {
		return "", fmt.Errorf("无 token")
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := m.client.Do(req)
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

	var result []struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("解析失败")
	}
	if len(result) == 0 || len(result[0].Translations) == 0 {
		return "", fmt.Errorf("empty microsoft result")
	}
	return result[0].Translations[0].Text, nil
}

// ==================== DeepLX 免费接口（无 Key） ====================
type DeepLXTranslator struct {
	client  *http.Client
	baseURL string
}

func NewDeepLXTranslator(baseURL string) *DeepLXTranslator {
	if baseURL == "" {
		// 常用公共实例（可自行替换更稳定的）
		baseURL = "https://deeplx.vercel.app"
	}
	return &DeepLXTranslator{
		client:  &http.Client{Timeout: 12 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (d *DeepLXTranslator) Name() string { return "DeepLX" }

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

	req, err := http.NewRequest("POST", d.baseURL+"/translate", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
	return result.Data, nil
}

// ==================== 有道免费网页接口（无 Key） ====================
type YoudaoTranslator struct {
	client *http.Client
}

func NewYoudaoTranslator() *YoudaoTranslator {
	return &YoudaoTranslator{client: &http.Client{Timeout: 8 * time.Second}}
}

func (y *YoudaoTranslator) Name() string { return "Youdao" }

func (y *YoudaoTranslator) Translate(text, from, to string) (string, error) {
	// 有道公开接口（有频率限制，适合备用）
	form := url.Values{}
	form.Set("i", text)
	form.Set("from", langMap(from))
	form.Set("to", langMap(to))
	form.Set("doctype", "json")
	form.Set("version", "2.1")
	form.Set("keyfrom", "fanyi.web")
	form.Set("action", "FY_BY_REALTIME")
	form.Set("typoResult", "false")

	req, err := http.NewRequest("POST", "https://fanyi.youdao.com/translate?smartresult=dict&smartresult=rule", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://fanyi.youdao.com/")
	req.Header.Set("Origin", "https://fanyi.youdao.com")

	resp, err := y.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := rejectNonJSON(data); err != nil {
		return "", err
	}
	var result struct {
		TranslateResult [][]struct {
			Tgt string `json:"tgt"`
			Src string `json:"src"`
		} `json:"translateResult"`
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("解析失败")
	}
	if result.ErrorCode != "0" && result.ErrorCode != "" {
		return "", fmt.Errorf("youdao errorCode %s", result.ErrorCode)
	}
	if len(result.TranslateResult) == 0 || len(result.TranslateResult[0]) == 0 {
		return "", fmt.Errorf("empty youdao result")
	}
	return result.TranslateResult[0][0].Tgt, nil
}

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
	api := "https://translate.googleapis.com/translate_a/single?" + q.Encode()
	req, err := http.NewRequest("GET", api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
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

func langMap(lang string) string {
	switch strings.ToLower(lang) {
	case "zh", "zh-cn", "chinese":
		return "zh-CHS"
	case "kor", "ko", "korean":
		return "ko"
	case "en", "eng", "english":
		return "en"
	case "auto", "":
		return "AUTO"
	default:
		return lang
	}
}

// ==================== 多引擎自动回退（开箱即用） ====================
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
			NewMicrosoftTranslator(),
			NewDeepLXTranslator(""),
			NewYoudaoTranslator(),
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

// Translate 自动尝试所有无 Key 引擎，成功即返回（带短缓存，避免同一句反复打接口）。
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

	var lastErr error
	for _, eng := range m.engines {
		result, err := eng.Translate(text, from, to)
		if err == nil && result != "" {
			m.mu.Lock()
			if len(m.order) >= 400 {
				old := m.order[0]
				m.order = m.order[1:]
				delete(m.cache, old)
			}
			m.cache[key] = result
			m.order = append(m.order, key)
			m.mu.Unlock()
			return result, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", fmt.Errorf("翻译失败")
	}
	return "", fmt.Errorf("翻译失败")
}

func (m *Manager) Name() string {
	return "Multi-Engine (No-Key)"
}
