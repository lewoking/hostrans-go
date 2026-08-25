package memory

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	colorTagRe = regexp.MustCompile(`(?i)</?c\b[^>]*>`)
	anyTagRe   = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	channelRe  = regexp.MustCompile(`^\s*[\[【]([^\]】]+)[\]】]\s*`)
	hangulRe   = regexp.MustCompile(`[\x{AC00}-\x{D7AF}\x{1100}-\x{11FF}\x{3130}-\x{318F}]`)
)

var uiNoise = []string{
	"综合 한국어",
	"浏览战利",
	"浏览收藏",
	"菜单",
	"메뉴",
	"Heroes of the Storm",
	"《风暴英雄》",
	"히어로즈 오브 더 스톰",
}

var channelNames = map[string]struct{}{
	"团队": {}, "所有人": {}, "综合": {}, "密语": {}, "队伍": {},
	"all": {}, "team": {}, "whisper": {}, "party": {},
	"팀": {}, "전체": {}, "귓속말": {}, "일반": {},
}

// ChatLine 解析后的一条聊天
type ChatLine struct {
	Raw     string
	Channel string
	Speaker string
	Text    string
}

func stripTags(s string) string {
	s = colorTagRe.ReplaceAllString(s, "")
	s = anyTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(s)
}

// ParseChatLine 从内存原文提取说话人和正文。
func ParseChatLine(raw string) ChatLine {
	line := ChatLine{Raw: raw}
	s := stripTags(raw)
	if s == "" {
		return line
	}
	// 多行时取最后一条有内容的
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		rest := strings.TrimSpace(s[i+1:])
		if rest != "" {
			s = rest
		}
	}

	if m := channelRe.FindStringSubmatch(s); m != nil {
		line.Channel = strings.TrimSpace(m[1])
		s = strings.TrimSpace(s[len(m[0]):])
	}

	s = strings.TrimLeft(s, ":： ")
	if s == "" {
		return line
	}

	if i := strings.IndexAny(s, ":："); i > 0 {
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+1:])
		if isSpeaker(left) && right != "" {
			line.Speaker = left
			line.Text = right
			return line
		}
	}

	line.Text = s
	return line
}

func isSpeaker(s string) bool {
	if s == "" {
		return false
	}
	if _, ok := channelNames[strings.ToLower(s)]; ok {
		return false
	}
	n := utf8.RuneCountInString(s)
	if n == 0 || n > 24 {
		return false
	}
	for _, r := range s {
		if r == '\n' || r == '\r' {
			return false
		}
	}
	return true
}

// ShouldSkip 过滤 UI 占位、空行、自己发出的探测串等。
func ShouldSkip(line ChatLine, myProbes map[string]struct{}) bool {
	s := stripTags(line.Raw)
	if s == "" {
		return true
	}
	for _, n := range uiNoise {
		if strings.Contains(s, n) {
			return true
		}
	}
	body := line.Text
	if body == "" {
		body = s
	}
	if strings.TrimSpace(body) == "" {
		return true
	}
	if myProbes != nil {
		if _, ok := myProbes[body]; ok {
			return true
		}
		if _, ok := myProbes[s]; ok {
			return true
		}
	}
	// 频道标签本身，没有正文
	if line.Speaker == "" && line.Text == "" {
		return true
	}
	return false
}

func ContainsKorean(s string) bool {
	return hangulRe.MatchString(s)
}

func ContainsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// NeedsTranslate 自动监控只翻译含韩文的发言（英文不译）。
func NeedsTranslate(text string) bool {
	return ContainsKorean(strings.TrimSpace(text))
}

func DisplaySpeaker(line ChatLine) string {
	if line.Speaker != "" {
		return line.Speaker
	}
	return ""
}
