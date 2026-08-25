package translator

import (
	"strings"
	"unicode"
)

func hasHangul(s string) bool {
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7AF || r >= 0x1100 && r <= 0x11FF || r >= 0x3130 && r <= 0x318F {
			return true
		}
	}
	return false
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// AcceptTranslation 拒绝原样返回、以及没写成目标语言的结果。
func AcceptTranslation(src, dst, to string) bool {
	src = strings.TrimSpace(src)
	dst = strings.TrimSpace(dst)
	if dst == "" {
		return false
	}
	if src != "" && dst == src {
		return false
	}
	switch strings.ToLower(to) {
	case "ko", "kor", "korean":
		return hasHangul(dst)
	case "zh", "zh-cn", "zh-hans", "chinese":
		return hasHan(dst)
	default:
		return true
	}
}
