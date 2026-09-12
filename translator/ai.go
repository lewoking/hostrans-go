package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hostrans/dlog"
	"io"
	"net/http"
	"strings"
	"time"
)

// 由 ldflags 注入，源码里保持为空。
var (
	AIKey   string
	AIBase  = "https://hub.oaifree.com"
	AIModel = "gpt-5.6-luna"
)

type AITranslator struct {
	client *http.Client
	base   string
	model  string
	key    string
}

func NewAITranslator() *AITranslator {
	base := strings.TrimRight(strings.TrimSpace(AIBase), "/")
	if base == "" {
		base = "https://hub.oaifree.com"
	}
	model := strings.TrimSpace(AIModel)
	if model == "" {
		model = "gpt-5.6-luna"
	}
	return &AITranslator{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        8,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		base:  base,
		model: model,
		key:   strings.TrimSpace(AIKey),
	}
}

func (a *AITranslator) Warmup() {
	if a.key == "" {
		return
	}
	req, err := http.NewRequest("GET", a.base+"/v1/models", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	resp, err := a.client.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
}

func langName(lang string) string {
	switch strings.ToLower(lang) {
	case "zh", "zh-cn", "zh-hans", "chinese":
		return "简体中文"
	case "ko", "kor", "korean":
		return "韩语"
	case "en", "eng", "english":
		return "英语"
	default:
		return lang
	}
}

func buildAIInput(text, from, to string) string {
	// 明确禁止解释，避免模型把翻译过程或说明写入输出，增加计费 token。
	if from == "" || strings.EqualFold(from, "auto") {
		return "auto->" + to + "\n" + text + "\nOutput translation only."
	}
	return from + "->" + to + "\n" + text + "\nOutput translation only."
}

type responsesReq struct {
	Model           string `json:"model"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

type responsesResp struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Output []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func parseResponses(data []byte) (string, error) {
	if err := rejectNonJSON(data); err != nil {
		return "", err
	}
	var r responsesResp
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("解析失败")
	}
	if r.Error.Message != "" {
		return "", fmt.Errorf("%s", r.Error.Message)
	}
	var b strings.Builder
	for _, item := range r.Output {
		for _, c := range item.Content {
			if c.Text != "" {
				b.WriteString(c.Text)
			}
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.Trim(out, "\"“”")
	if out == "" {
		return "", fmt.Errorf("empty")
	}
	return out, nil
}

func (a *AITranslator) Translate(text, from, to string) (string, error) {
	if a.key == "" {
		return "", fmt.Errorf("未注入翻译密钥")
	}
	input := buildAIInput(text, from, to)
	payload, err := json.Marshal(responsesReq{
		Model:           a.model,
		Input:           input,
		MaxOutputTokens: 256,
	})
	// 仅记录实际 JSON 大小和 input；payload 不记录，避免泄露 Authorization 密钥。
	dlog.Infof("AI request model=%q input_bytes=%d payload_bytes=%d input=%q", a.model, len(input), len(payload), input)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", a.base+"/v1/responses", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", httpUserAgent)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out, err := parseResponses(data)
	if resp.StatusCode != 200 {
		if err != nil {
			return "", fmt.Errorf("http %d: %v", resp.StatusCode, err)
		}
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	return out, err
}
