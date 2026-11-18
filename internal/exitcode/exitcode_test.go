package exitcode

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestExitcodeOf(t *testing.T) {
	base := errors.New("x")
	for _, tc := range []struct {
		err  error
		want int
	}{
		{nil, 0},
		{fmt.Errorf("stop: %w", context.Canceled), 130},
		{base, 1},
		{UsageErr(base), 2},
		{Authf("no"), 3},
		{NotFoundf("gone"), 4},
		{Conflictf("busy"), 5},
		{WithHint(NotFoundf("gone"), "try this"), 4},
		{FromHTTP(404, base), 4},
		{FromHTTP(500, base), 1},
	} {
		if got := Of(tc.err); got != tc.want {
			t.Errorf("Of(%v) = %d, want %d", tc.err, got, tc.want)
		}
	}
	if got := Hint(UsageErr(WithHint(base, "h"))); got != "h" {
		t.Errorf("hint lost through a code: %q", got)
	}
}
