package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	seen := map[string]struct{}{}
	for _, blob := range blobs {
		for _, piece := range splitLines(blob) {
			if !LooksLikeChat(piece) {
				continue
			}
			line := ParseChatLine(piece)
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
			if _, ok := seen[body]; ok {
				continue
			}
			seen[body] = struct{}{}
			if line.Text == "" {
				line.Text = body
			}
			out = append(out, line)
		}
	}
	return out
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
