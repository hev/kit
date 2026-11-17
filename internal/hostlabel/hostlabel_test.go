package hostlabel

import "testing"

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"Adams-MacBook-Air":       "macbook",
		"Adams-MacBook-Air.local": "macbook",
		"adams-macbook-pro":       "macbook",
		"Adams-Mac-mini.local":    "mini",
		"Oven":                    "oven",
		"win-oven.lan":            "win-oven",
		"":                        "",
	} {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelfPrefersPeersSelf(t *testing.T) {
	host := func() (string, error) { return "Adams-MacBook-Air.local", nil }
	t.Setenv("PEERS_SELF", "")
	if got := Self(host); got != "macbook" {
		t.Fatalf("Self = %q, want macbook", got)
	}
	t.Setenv("PEERS_SELF", "Custom")
	if got := Self(host); got != "Custom" {
		t.Fatalf("Self with PEERS_SELF = %q, want Custom", got)
	}
}

func TestAddressBackCompat(t *testing.T) {
	for in, want := range map[string]string{
		"Adams-MacBook-Air/foh-hev-6": "macbook/foh-hev-6",
		"adams-macbook-air/foh-hev-6": "macbook/foh-hev-6",
		"ADAMS-MACBOOK-AIR/foh-hev-6": "macbook/foh-hev-6",
		"Adams-Mac-mini/boh-1":        "mini/boh-1",
		"macbook/foh-hev-6":           "macbook/foh-hev-6",
		"Oven/foh-1":                  "Oven/foh-1",
		"foh-1":                       "foh-1",
	} {
		if got := Address(in); got != want {
			t.Errorf("Address(%q) = %q, want %q", in, got, want)
		}
	}
	if !SameAddress("Adams-MacBook-Air/foh-hev-6", "macbook/foh-hev-6") {
		t.Fatal("legacy laptop address does not match macbook")
	}
	if SameAddress("macbook/foh-hev-6", "macbook/foh-hev-7") || SameAddress("mini/foh-1", "macbook/foh-1") {
		t.Fatal("distinct addresses matched")
	}
}
