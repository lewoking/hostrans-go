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
	hangulRe = regexp.MustCompile(`[\x{AC00}-\x{D7AF}\x{1100}-\x{11FF}\x{3130}-\x{318F}]`)
	// 剥标签后只认两种：频道]名字:正文  或  频道]:正文。不含 <>% 。
	chatNamedRe = regexp.MustCompile(`^[\[【]?(征召团队|征召队伍|团队|房间|队伍|组队|所有人|综合|팀|전체)[\]】]\s*([^<>%\[\]【】\n:：]{1,24})\s*[:：]\s*([^<>%\n]+)$`)
	chatPlainRe = regexp.MustCompile(`^[\[【]?(征召团队|征召队伍|团队|房间|队伍|组队|所有人|综合|팀|전체)[\]】]\s*[:：]\s*([^<>%\n]+)$`)
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
	"团队": {}, "征召团队": {}, "征召队伍": {}, "征召": {}, "房间": {}, "所有人": {}, "综合": {}, "密语": {}, "队伍": {}, "组队": {},
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
	s = unescapeChat(s)
	return strings.TrimSpace(s)
}

func unescapeChat(s string) string {
	r := strings.NewReplacer(
		"&apos;", "'",
		"&#39;", "'",
		"&quot;", `"`,
		"&nbsp;", " ",
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
	)
	return r.Replace(s)
}

// ParseChatLine 转码剥标签后，正则只拆干净的 频道]名字:内容。
func ParseChatLine(raw string) ChatLine {
	line := ChatLine{Raw: raw}
	s := stripTags(raw)
	if s == "" {
		return line
	}
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		rest := strings.TrimSpace(s[i+1:])
		if rest != "" {
			s = rest
		}
	}
	if m := chatNamedRe.FindStringSubmatch(s); m != nil {
		line.Channel = m[1]
		line.Speaker = strings.TrimSpace(m[2])
		line.Text = strings.TrimSpace(m[3])
		return line
	}
	if m := chatPlainRe.FindStringSubmatch(s); m != nil {
		line.Channel = m[1]
		line.Text = strings.TrimSpace(m[2])
		return line
	}
	return line
}

func KnownChannel(s string) bool {
	if s == "" {
		return false
	}
	_, ok := channelNames[strings.ToLower(s)]
	if ok {
		return true
	}
	_, ok = channelNames[s]
	return ok
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
		if r == '\n' || r == '\r' || r == '\uFFFD' || r == '%' {
			return false
		}
	}
	return true
}

func isTemplateText(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "%senderName%") || strings.Contains(s, "%message%") || strings.Contains(s, "%presenceId%") {
		return true
	}
	if strings.Contains(s, "storm_ui_") || strings.Contains(s, ".dds") {
		return true
	}
	return strings.Contains(s, "%")
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
	if isTemplateText(s) || isTemplateText(body) || isTemplateText(line.Speaker) {
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

func DisplayWho(line ChatLine) string {
	var b strings.Builder
	if line.Channel != "" {
		b.WriteString(line.Channel)
		b.WriteString("]")
	}
	if line.Speaker != "" {
		b.WriteString(line.Speaker)
	}
	return b.String()
}
