package layer

import "testing"

func TestNativeHostnameNormalization(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"WIN-OVEN", "win-oven"}, {"WIN-OVEN.example.test", "win-oven"}, {"mini.local", "mini"}, {"oven", "oven"}} {
		if got := normalizeHostname(tc.input); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.input, got, tc.want)
		}
	}
}
