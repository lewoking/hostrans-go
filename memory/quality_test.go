package memory

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLooksLikeChat(t *testing.T) {
	ok := []string{
		"이길 수 없다",
		"[团队] honghuli: 이길 수 없다",
		"honghuli: 만나서 반갑습니다.",
	}
	for _, s := range ok {
		if !LooksLikeChat(s) {
			t.Errorf("LooksLikeChat(%q) = false", s)
		}
	}
	bad := []string{
		"",
		"[团队]:",
		"<c val=\"3184FF\">[团队]:</c>",
		"浏览战利品",
		string([]byte{0xaa, 0xa7, 0x31, 0x88, 0xaa, 0xaa, 0xea, 0xb0, 0x80}),
		strings.Repeat("L ", 40) + "ֽ",
	}
	for _, s := range bad {
		if LooksLikeChat(s) {
			t.Errorf("LooksLikeChat(%q) = true", s)
		}
	}
}

func TestExtractUTF8Strings(t *testing.T) {
	raw := append([]byte("[团队]:"), 0)
	raw = append(raw, []byte("honghuli: 이길 수 없다")...)
	raw = append(raw, 0, 0xaa, 0xa7, 0x31, 0)
	got := ExtractUTF8Strings(raw)
	found := false
	for _, s := range got {
		if strings.Contains(s, "이길 수 없다") {
			found = true
		}
		if !utf8.ValidString(s) {
			t.Fatalf("invalid utf8 extracted: %q", s)
		}
	}
	if !found {
		t.Fatalf("missing hangul chat, got %#v", got)
	}
}

func TestParseChatLineSpeakerAfterChannel(t *testing.T) {
	got := ParseChatLine("[团队] honghuli: 이길 수 없다")
	if got.Speaker != "honghuli" || got.Text != "이길 수 없다" {
		t.Fatalf("got speaker=%q text=%q", got.Speaker, got.Text)
	}
}

func TestChatCandidatesFindsHangulNotPrefix(t *testing.T) {
	raw := "[团队]:\x00honghuli: 이길 수 없다"
	cands := ChatCandidates(raw)
	if len(cands) != 1 || cands[0].Speaker != "honghuli" || cands[0].Text != "이길 수 없다" {
		t.Fatalf("cands=%+v", cands)
	}
}
