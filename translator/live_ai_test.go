package translator

import "testing"

func TestLiveAI(t *testing.T) {
	a := NewAITranslator()
	if !a.Ready() {
		t.Skip("AI key not injected")
	}
	got, err := a.Translate("대왕이 왔어", "ko", "zh")
	if err != nil {
		t.Fatal(err)
	}
	if !AcceptTranslation("대왕이 왔어", got, "zh") {
		t.Fatalf("rejected %q", got)
	}
	t.Logf("대왕이 왔어 => %q", got)
}
