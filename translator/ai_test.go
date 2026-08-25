package translator

import (
	"strings"
	"testing"
)

func TestParseResponses(t *testing.T) {
	raw := []byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"大王来了"}]}]}`)
	got, err := parseResponses(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "大王来了" {
		t.Fatalf("got %q", got)
	}
}

func TestParseResponsesError(t *testing.T) {
	raw := []byte(`{"error":{"message":"Unauthorized"}}`)
	if _, err := parseResponses(raw); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildAIInput(t *testing.T) {
	in := buildAIInput("대왕이 왔어", "auto", "zh")
	if !strings.Contains(in, "简体中文") || !strings.Contains(in, "대왕이 왔어") {
		t.Fatalf("bad input %q", in)
	}
}

func TestParseChatCompletions(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"content":"大王来了"}}]}`)
	got, err := parseChatCompletions(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "大王来了" {
		t.Fatalf("got %q", got)
	}
}

func TestNewManagerEngines(t *testing.T) {
	m := NewManager()
	if len(m.engines) != 1 || m.engines[0].Name() != "AI" {
		t.Fatalf("engines=%v", m.engines)
	}
}

func TestAITranslatorNotReadyWithoutKey(t *testing.T) {
	old := AIKey
	AIKey = ""
	defer func() { AIKey = old }()
	a := NewAITranslator()
	if a.Ready() {
		t.Fatal("empty key should not be ready")
	}
	if _, err := a.Translate("대왕이 왔어", "ko", "zh"); err == nil {
		t.Fatal("expected missing key error")
	}
}
