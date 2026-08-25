package memory

import "strings"

type InputKind int

const (
	InputEmpty InputKind = iota
	InputChinese
	InputKorean
	InputOther
)

func ClassifyInput(s string) InputKind {
	s = strings.TrimSpace(s)
	if s == "" {
		return InputEmpty
	}
	if ContainsHan(s) {
		return InputChinese
	}
	if ContainsKorean(s) {
		return InputKorean
	}
	return InputOther
}

// IsChannelPrefixOnly 输入框左侧的 [团队]: 之类，不是一条聊天。
func IsChannelPrefixOnly(raw string) bool {
	line := ParseChatLine(raw)
	return strings.TrimSpace(line.Text) == "" && strings.TrimSpace(line.Speaker) == ""
}
