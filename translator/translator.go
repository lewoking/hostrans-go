package translator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

func (m *MicrosoftTranslator) Translate(text, from, to string) (string, error) {
	// 微软免费公共接口（edge 风格）
	// 实际生产中常用的是通过 edge 的翻译 token 接口，这里先用简化可用版本
	apiURL := "https://api-edge.cognitive.microsofttranslator.com/translate?api-version=3.0&to=" + to
	if from != "" && from != "auto" {
		apiURL += "&from=" + from
	}

	body := fmt.Sprintf(`[{"Text":%q}]`, text)
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("microsoft %d: %s", resp.StatusCode, string(data))
	}

	var result []struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
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

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Code    int    `json:"code"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("deeplx parse: %v", err)
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

	data, _ := io.ReadAll(resp.Body)
	var result struct {
		TranslateResult [][]struct {
			Tgt string `json:"tgt"`
			Src string `json:"src"`
		} `json:"translateResult"`
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.ErrorCode != "0" && result.ErrorCode != "" {
		return "", fmt.Errorf("youdao errorCode %s", result.ErrorCode)
	}
	if len(result.TranslateResult) == 0 || len(result.TranslateResult[0]) == 0 {
		return "", fmt.Errorf("empty youdao result")
	}
	return result.TranslateResult[0][0].Tgt, nil
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
}

func NewManager() *Manager {
	return &Manager{
		engines: []Translator{
			NewMicrosoftTranslator(),
			NewDeepLXTranslator(""),
			NewYoudaoTranslator(),
		},
	}
}

// Translate 自动尝试所有无 Key 引擎，成功即返回
func (m *Manager) Translate(text, from, to string) (string, error) {
	if text == "" {
		return "", nil
	}
	var lastErr error
	for _, eng := range m.engines {
		result, err := eng.Translate(text, from, to)
		if err == nil && result != "" {
			return result, nil
		}
		lastErr = fmt.Errorf("%s: %v", eng.Name(), err)
	}
	return "", fmt.Errorf("全部引擎失败: %v", lastErr)
}

func (m *Manager) Name() string {
	return "Multi-Engine (No-Key)"
}
