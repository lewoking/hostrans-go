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

const (
	defaultAIBase  = "https://gateway.ai.cloudflare.com/v1/64d53ca476db7004bc2b51e1d9db2dad/translation/compat"
	defaultAIModel = "dynamic/free"
	maxOutTokens   = 256
)

// 由 ldflags 注入，源码里密钥保持为空。
var (
	AIKey   string
	AIBase  = defaultAIBase
	AIModel = defaultAIModel
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
		base = defaultAIBase
	}
	model := strings.TrimSpace(AIModel)
	if model == "" {
		model = defaultAIModel
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

func (a *AITranslator) setAuth(req *http.Request) {
	token := "Bearer " + a.key
	if strings.Contains(a.base, "gateway.ai.cloudflare.com") {
		req.Header.Set("cf-aig-authorization", token)
		return
	}
	req.Header.Set("Authorization", token)
}

func (a *AITranslator) Warmup() {
	if a.key == "" {
		return
	}
	req, err := http.NewRequest("GET", a.base+"/models", nil)
	if err != nil {
		return
	}
	a.setAuth(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
}

func buildAIInput(text, from, to string) string {
	// 明确禁止解释，避免模型把翻译过程或说明写入输出，增加计费 token。
	if from == "" || strings.EqualFold(from, "auto") {
		return "auto->" + to + "\n" + text + "\nOutput translation only."
	}
	return from + "->" + to + "\n" + text + "\nOutput translation only."
}

type thinkingOpt struct {
	Type string `json:"type"`
}

type chatReq struct {
	Model              string         `json:"model"`
	Messages           []chatMessage  `json:"messages"`
	MaxTokens          int            `json:"max_tokens,omitempty"`
	Thinking           *thinkingOpt   `json:"thinking,omitempty"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
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
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func contentText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return strings.TrimSpace(s)
		}
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return strings.TrimSpace(b.String())
	}
	return ""
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
	var out string
	for _, c := range r.Choices {
		if t := contentText(c.Message.Content); t != "" {
			out = t
			break
		}
	}
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
	payload, err := json.Marshal(chatReq{
		Model: a.model,
		Messages: []chatMessage{
			{Role: "user", Content: input},
		},
		MaxTokens: maxOutTokens,
		// Gateway 路由节点没有关思考开关；Workers AI 的 GLM 默认会把 token 花在 reasoning 上，content 变 null。
		Thinking: &thinkingOpt{Type: "disabled"},
		ChatTemplateKwargs: map[string]any{
			"enable_thinking": false,
		},
	})
	// 仅记录实际 JSON 大小和 input；payload 不记录，避免泄露密钥。
	dlog.Infof("AI request model=%q input_bytes=%d payload_bytes=%d input=%q", a.model, len(input), len(payload), input)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", a.base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	a.setAuth(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", httpUserAgent)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out, err := parseChatCompletions(data)
	if resp.StatusCode != 200 {
		if err != nil {
			return "", fmt.Errorf("http %d: %v", resp.StatusCode, err)
		}
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	return out, err
}
