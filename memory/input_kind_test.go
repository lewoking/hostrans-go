package memory

import "testing"

func TestClassifyInput(t *testing.T) {
	cases := []struct {
		in   string
		want InputKind
	}{
		{"", InputEmpty},
		{"   ", InputEmpty},
		{"你好啊", InputChinese},
		{"[团队]:你好", InputChinese},
		{"안녕하세요", InputKorean},
		{"대장이 왔이", InputKorean},
		{"gg", InputOther},
	}
	for _, c := range cases {
		if got := ClassifyInput(c.in); got != c.want {
			t.Errorf("ClassifyInput(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIsChannelPrefixOnly(t *testing.T) {
	if !IsChannelPrefixOnly(`[团队]:`) {
		t.Fatal("plain channel tag should be prefix-only")
	}
	if !IsChannelPrefixOnly(`<c val="3184FF">[团队]:</c>`) {
		t.Fatal("html channel tag should be prefix-only")
	}
	if IsChannelPrefixOnly(`[团队] t66y: 대장이 왔이`) {
		t.Fatal("real chat line is not prefix-only")
	}
	if IsChannelPrefixOnly(`홍길동: 안녕하세요`) {
		t.Fatal("speaker line is not prefix-only")
	}
}
