package translator

import "testing"

func TestAcceptTranslation(t *testing.T) {
	if AcceptTranslation("你好啊", "你好啊", "ko") {
		t.Fatal("zh→ko identity should reject")
	}
	if AcceptTranslation("안녕하세요", "안녕하세요", "zh") {
		t.Fatal("ko→zh identity should reject")
	}
	if !AcceptTranslation("你好", "안녕", "ko") {
		t.Fatal("zh→ko hangul should accept")
	}
	if !AcceptTranslation("안녕하세요", "你好", "zh") {
		t.Fatal("ko→zh han should accept")
	}
	if AcceptTranslation("你好", "hello", "ko") {
		t.Fatal("zh→ko without hangul should reject")
	}
}
