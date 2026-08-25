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
	AIFast  = "gpt-5.6-sol"
)

type AITranslator struct {
	client       *http.Client
	base         string
	model        string
	key          string
	name         string
	useResponses bool
}

func sharedAIClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        8,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func NewAITranslator() *AITranslator {
	return newAI("AI", strings.TrimSpace(AIModel), true)
}

func NewFastAITranslator() *AITranslator {
	model := strings.TrimSpace(AIFast)
	if model == "" {
		model = "gpt-5.6-sol"
	}
	return newAI("AI-fast", model, false)
}

func newAI(name, model string, responses bool) *AITranslator {
	base := strings.TrimRight(strings.TrimSpace(AIBase), "/")
	if base == "" {
		base = "https://hub.oaifree.com"
	}
	if model == "" {
		model = "gpt-5.6-luna"
	}
	return &AITranslator{
		client:       sharedAIClient(),
		base:         base,
		model:        model,
		key:          strings.TrimSpace(AIKey),
		name:         name,
		useResponses: responses,
	}
}

func (a *AITranslator) Name() string { return a.name }

func (a *AITranslator) Ready() bool { return a.key != "" }

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

type chatReq struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
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
	return cleanAIOut(b.String())
}

func parseChatCompletions(data []byte) (string, error) {
	if err := rejectNonJSON(data); err != nil {
		return "", err
	}
	var r chatResp
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("解析失败")
	}
	if r.Error.Message != "" {
		return "", fmt.Errorf("%s", r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("empty")
	}
	return cleanAIOut(r.Choices[0].Message.Content)
}

func cleanAIOut(s string) (string, error) {
	out := strings.TrimSpace(s)
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
	prompt := buildAIInput(text, from, to)
	var (
		api  string
		body []byte
		err  error
	)
	if a.useResponses {
		api = a.base + "/v1/responses"
		body, err = json.Marshal(responsesReq{Model: a.model, Input: prompt, MaxOutputTokens: 64})
	} else {
		api = a.base + "/v1/chat/completions"
		body, err = json.Marshal(chatReq{
			Model:       a.model,
			Temperature: 0,
			MaxTokens:   64,
			Messages:    []chatMessage{{Role: "user", Content: prompt}},
		})
	}
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", api, bytes.NewReader(body))
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
	var out string
	if a.useResponses {
		out, err = parseResponses(data)
	} else {
		out, err = parseChatCompletions(data)
	}
	if resp.StatusCode != 200 {
		if err != nil {
			return "", fmt.Errorf("http %d: %v", resp.StatusCode, err)
		}
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	return out, err
}
