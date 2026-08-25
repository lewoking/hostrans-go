package translator

import (
	"bytes"
	"encoding/json"
	"fmt"
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
				MaxIdleConns:        4,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		base:  base,
		model: model,
		key:   strings.TrimSpace(AIKey),
	}
}

func (a *AITranslator) Name() string { return "AI" }

func (a *AITranslator) Ready() bool { return a.key != "" }

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
	dst := langName(to)
	src := langName(from)
	if from == "" || strings.EqualFold(from, "auto") {
		return "把下面游戏聊天译成" + dst + "，只输出译文：\n" + text
	}
	return "把下面" + src + "游戏聊天译成" + dst + "，只输出译文：\n" + text
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
	payload, err := json.Marshal(responsesReq{
		Model:           a.model,
		Input:           buildAIInput(text, from, to),
		MaxOutputTokens: 64,
	})
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
	if resp.StatusCode != 200 {
		if _, perr := parseResponses(data); perr != nil && strings.Contains(perr.Error(), " ") {
			return "", fmt.Errorf("http %d: %v", resp.StatusCode, perr)
		}
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	return parseResponses(data)
}
