package layer

import "testing"

func TestNativeHostnameNormalization(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"WIN-DESKTOP", "win-desktop"}, {"WIN-DESKTOP.example.test", "win-desktop"}, {"laptop.local", "laptop"}, {"workstation", "workstation"}} {
		if got := normalizeHostname(tc.input); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.input, got, tc.want)
		}
	}
}
