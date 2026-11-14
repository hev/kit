package peersview_test

import (
	"strings"
	"testing"
)

func TestReviewerCapTailAllContexts(t *testing.T) {
	v := strings.Repeat("}", 16384) + "UNINSPECTED"
	b := deltaRender(t, v)
	if strings.Contains(b, "UNINSPECTED") {
		t.Fatal("tail displayed")
	}
	if n := strings.Count(b, "[truncated]"); n != 18 {
		t.Fatalf("markers=%d want 18", n)
	}
}
func TestReviewerCapBoundaryAllContexts(t *testing.T) {
	// The drive path starts one byte before the cap and continues beyond it.
	v := strings.Repeat("}", 16383) + `C:\Users\boundary-private.txt`
	b := deltaRender(t, v)
	if n := strings.Count(b, "C[truncated]"); n != 0 {
		t.Fatalf("partial drive path C displayed in %d contexts (want 0); input=16383 closing braces + C:\\Users\\boundary-private.txt", n)
	}
}
func TestReviewerExactClosingInputs(t *testing.T) {
	for _, v := range []string{`label:/home/x/private.txt`, `log:/Users/x/a`, `path=/home/x`, `at:C:\Users\x`, `see:~/notes`, `%20%2Fhome%2Fx%20`, `+%2Fhome%2Fx+`, `%20C%3A%5CUsers%5Cx`} {
		if b := deltaRender(t, v); strings.Contains(b, v) {
			t.Errorf("displayed %q", v)
		}
	}
}

func TestReviewerStraddlingTokensAllContexts(t *testing.T) {
	for _, token := range []string{`/home/x/private.txt`, `~/notes`, `file:///home/x`, `label:/home/x`, `%2Fhome%2Fx%2Fprivate.txt`, `界private`} {
		v := strings.Repeat("}", 16383) + token
		b := deltaRender(t, v)
		want := strings.Repeat("}", 16383) + "[truncated]"
		if n := strings.Count(b, want); n != 18 {
			t.Errorf("token %q: whole-token truncation in %d contexts, want 18", token, n)
		}
	}
}
func TestReviewerExactCapAllContexts(t *testing.T) {
	for _, n := range []int{16384, 16385} {
		v := strings.Repeat("x", n)
		want := v
		if n > 16384 {
			want = "[truncated]"
		}
		if got := strings.Count(deltaRender(t, v), want); got != 18 {
			t.Errorf("length %d: %d contexts want 18", n, got)
		}
	}
}
