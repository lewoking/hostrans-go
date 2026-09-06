package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// WindowToString 把一段内存窗解成可抽句的文本。NUL 改成换行，不在第一个 0 处截断。
func WindowToString(data []byte, enc string) string {
	if len(data) == 0 {
		return ""
	}
	if enc == "utf-16le" || enc == "utf-16" {
		var b strings.Builder
		b.Grow(len(data) / 2)
		for i := 0; i+1 < len(data); i += 2 {
			r := rune(uint16(data[i]) | uint16(data[i+1])<<8)
			if r == 0 {
				b.WriteByte('\n')
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	return strings.ReplaceAll(string(data), "\x00", "\n")
}

// ExtractUTF8Strings 按 0x00 切开窗口里的 C 字符串，丢掉非法 UTF-8。
func ExtractUTF8Strings(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	parts := strings.Split(string(data), "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || !utf8.ValidString(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func LooksLikeChat(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) {
		return false
	}
	n := utf8.RuneCountInString(s)
	if n == 0 || n > 200 {
		return false
	}
	if strings.ContainsRune(s, '\uFFFD') {
		return false
	}
	if IsChannelPrefixOnly(s) {
		return false
	}
	print, hangul, other := 0, 0, 0
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			print++
			continue
		}
		if unicode.IsPrint(r) {
			print++
			if r >= 0xAC00 && r <= 0xD7AF {
				hangul++
			}
			continue
		}
		other++
	}
	if other > 0 || print < n {
		return false
	}
	if hangul == 0 {
		return false
	}
	// 乱码里偶尔也会混到韩文音节，真实聊天韩文占比通常更高
	if hangul*8 < n && n > 24 {
		return false
	}
	for _, noise := range uiNoise {
		if strings.Contains(s, noise) {
			return false
		}
	}
	return true
}

// ChatCandidates 从一块原始内存里抽出可翻译的韩文聊天。
func ChatCandidates(raw string) []ChatLine {
	blobs := ExtractUTF8Strings(append([]byte(raw), 0))
	if len(blobs) == 0 {
		blobs = []string{raw}
	}
	var out []ChatLine
	idx := map[string]int{}
	lastName := ""
	for _, blob := range blobs {
		for _, piece := range splitLines(blob) {
			line := ParseChatLine(piece)
			if line.Speaker != "" {
				lastName = line.Speaker
			} else if looksLikeName(piece) {
				lastName = piece
				continue
			}
			if !LooksLikeChat(piece) {
				continue
			}
			body := line.Text
			if body == "" {
				body = piece
			}
			if !NeedsTranslate(body) && !NeedsTranslate(piece) {
				continue
			}
			if body == "" {
				continue
			}
			if line.Speaker == "" && lastName != "" {
				line.Speaker = lastName
			}
			if line.Text == "" {
				line.Text = body
			}
			if i, ok := idx[body]; ok {
				if out[i].Speaker == "" && line.Speaker != "" {
					out[i] = line
				}
				continue
			}
			idx[body] = len(out)
			out = append(out, line)
		}
	}
	return out
}

func looksLikeName(s string) bool {
	s = strings.TrimSpace(stripTags(s))
	if !isSpeaker(s) {
		return false
	}
	if channelRe.MatchString(s) {
		return false
	}
	if strings.ContainsAny(s, ":： 	") {
		return false
	}
	if ContainsKorean(s) && (utf8.RuneCountInString(s) > 8 || strings.ContainsAny(s, ".,!?，。！？")) {
		return false
	}
	return true
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
