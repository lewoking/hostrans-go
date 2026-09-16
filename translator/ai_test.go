package translator

import "testing"

func TestParseChatCompletions(t *testing.T) {
	out, err := parseChatCompletions([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
	if err != nil || out != "你好" {
		t.Fatalf("string content: out=%q err=%v", out, err)
	}

	out, err = parseChatCompletions([]byte(`{"choices":[{"message":{"content":[{"text":"안녕"}]}}]}`))
	if err != nil || out != "안녕" {
		t.Fatalf("array content: out=%q err=%v", out, err)
	}

	if _, err := parseChatCompletions([]byte(`{"choices":[{"message":{"content":null}}]}`)); err == nil {
		t.Fatal("null content should fail")
	}
	if _, err := parseChatCompletions([]byte(`{"error":{"message":"nope"}}`)); err == nil || err.Error() != "nope" {
		t.Fatalf("error field: %v", err)
	}
}
