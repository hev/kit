package peersview

import (
	"strings"
	"testing"
)

func TestReviewMegabyteBraceCost(t *testing.T) {
	for _, n := range []int{65536, 262144, 1048576} {
		got := safe(strings.Repeat("}", n))
		want := strings.Repeat("}", maxDisplayBytes) + "[truncated]"
		if got != want {
			t.Fatalf("%d bytes: inspection cap/visible marker missing", n)
		}
	}
}
