package memory

import "testing"

func TestParseChatLine(t *testing.T) {
	cases := []struct {
		raw, speaker, text string
	}{
		{`<c val="3184FF">[团队]:</c>홍길동: 안녕하세요`, "홍길동", "안녕하세요"},
		{`[All]PlayerOne: gg wp`, "PlayerOne", "gg wp"},
		{`홍길동: 집결`, "홍길동", "집결"},
		{`<c val="3184FF">[团队]:</c>`, "", ""},
		{"hello world", "", "hello world"},
	}
	for _, c := range cases {
		got := ParseChatLine(c.raw)
		if got.Speaker != c.speaker || got.Text != c.text {
			t.Errorf("ParseChatLine(%q) = speaker=%q text=%q, want %q / %q",
				c.raw, got.Speaker, got.Text, c.speaker, c.text)
		}
	}
}

func TestShouldSkipAndNeedsTranslate(t *testing.T) {
	empty := ParseChatLine(`<c val="3184FF">[团队]:</c>`)
	if !ShouldSkip(empty, nil) {
		t.Fatal("empty channel tag should skip")
	}
	ui := ParseChatLine("浏览战利品")
	if !ShouldSkip(ui, nil) {
		t.Fatal("ui noise should skip")
	}
	ko := ParseChatLine("홍길동: 안녕하세요")
	if ShouldSkip(ko, nil) {
		t.Fatal("korean chat should not skip")
	}
	if !NeedsTranslate(ko.Text) {
		t.Fatal("korean needs translate")
	}
	if NeedsTranslate("大家好，集合中路") {
		t.Fatal("pure chinese should not translate")
	}
	probes := map[string]struct{}{"HT1234567890": {}}
	p := ParseChatLine("HT1234567890")
	if !ShouldSkip(p, probes) {
		t.Fatal("own probe should skip")
	}
}

func TestContainsKorean(t *testing.T) {
	if !ContainsKorean("hi 안녕") {
		t.Fatal("expected korean")
	}
	if ContainsKorean("hello 你好") {
		t.Fatal("no hangul")
	}
}
