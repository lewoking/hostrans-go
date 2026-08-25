package translator

import (
	"strings"
	"testing"
)

func TestGoogleParseSample(t *testing.T) {
	// translate.googleapis.com 典型结构：[[["안녕","你好",null,null,10]]]
	raw := []byte(`[[["안녕","你好",null,null,10]]]`)
	if err := rejectNonJSON(raw); err != nil {
		t.Fatal(err)
	}
}

func TestDeepLXDefaultURL(t *testing.T) {
	d := NewDeepLXTranslator("")
	if d.endpoint != deepLXDefaultURL {
		t.Fatalf("endpoint = %s, want %s", d.endpoint, deepLXDefaultURL)
	}
}

func probe(t *testing.T, name string, tr Translator, text, from, to string) (string, error) {
	t.Helper()
	got, err := tr.Translate(text, from, to)
	if err != nil {
		t.Logf("%s FAIL %s→%s %q: %v", name, from, to, text, err)
		return "", err
	}
	t.Logf("%s OK %s→%s %q => %q", name, from, to, text, got)
	if strings.TrimSpace(got) == "" {
		t.Errorf("%s empty result", name)
	}
	return got, nil
}

func TestLiveEngines(t *testing.T) {
	type caseT struct{ text, from, to string }
	cases := []caseT{
		{"你好", "zh", "ko"},
		{"안녕하세요", "ko", "zh"},
	}
	engines := []Translator{
		NewGoogleTranslator(),
		NewDeepLXTranslator(""),
	}
	anyOK := false
	for _, eng := range engines {
		engOK := true
		for _, c := range cases {
			if _, err := probe(t, eng.Name(), eng, c.text, c.from, c.to); err != nil {
				engOK = false
			}
		}
		if engOK {
			anyOK = true
		}
	}
	if !anyOK {
		t.Fatal("所有引擎都失败，翻译不可用")
	}
}

func TestLiveManager(t *testing.T) {
	m := NewManager()
	if len(m.engines) != 2 {
		t.Fatalf("engines = %d, want Google+DeepLX", len(m.engines))
	}
	if m.engines[0].Name() != "Google" || m.engines[1].Name() != "DeepLX" {
		t.Fatalf("engine order = %s, %s", m.engines[0].Name(), m.engines[1].Name())
	}
	ko, err := m.Translate("集合中路", "zh", "ko")
	if err != nil {
		t.Fatalf("中→韩: %v", err)
	}
	t.Logf("中→韩 集合中路 => %q", ko)
	zh, err := m.Translate("안녕하세요", "ko", "zh")
	if err != nil {
		t.Fatalf("韩→中: %v", err)
	}
	t.Logf("韩→中 안녕하세요 => %q", zh)
}
