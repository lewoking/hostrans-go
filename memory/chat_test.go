package memory

import "testing"

func TestParseChatLineSupportsMonitoredChannels(t *testing.T) {
	tests := []struct {
		raw     string
		channel string
		speaker string
		text    string
	}{
		{"团队] Alice: hello", "团队", "Alice", "hello"},
		{"房间] 한국유저: hello", "房间", "한국유저", "hello"},
		{"征召团队] Player: 한국어", "征召团队", "Player", "한국어"},
	}

	for _, tt := range tests {
		line := ParseChatLine(tt.raw)
		if line.Channel != tt.channel || line.Speaker != tt.speaker || line.Text != tt.text {
			t.Fatalf("ParseChatLine(%q) = %#v", tt.raw, line)
		}
	}
}

func TestParseChatLineStopsAtNextRecord(t *testing.T) {
	line := ParseChatLine("团队] 한국유저: 안녕하세요征召团队] Alice: hello")
	if line.Channel != "团队" || line.Speaker != "한국유저" || line.Text != "안녕하세요" {
		t.Fatalf("ParseChatLine extracted %#v", line)
	}
}

func TestNeedsTranslateOnlyChecksMessageBody(t *testing.T) {
	line := ParseChatLine("团队] 한국유저: hello")
	if NeedsTranslate(line.Text) {
		t.Fatal("Korean speaker name must not make an English message translatable")
	}

	line = ParseChatLine("团队] Alice: 한국어")
	if !NeedsTranslate(line.Text) {
		t.Fatal("Korean message body must be translated")
	}
}
