package translator

import "testing"

func TestRejectNonJSON(t *testing.T) {
	if err := rejectNonJSON([]byte("<html>block</html>")); err == nil {
		t.Fatal("html should fail")
	}
	if err := rejectNonJSON([]byte(`[{"Text":"ok"}]`)); err != nil {
		t.Fatal(err)
	}
}
