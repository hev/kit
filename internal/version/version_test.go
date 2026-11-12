package version

import "testing"

// The release preflight runs this: a release binary must pull the dashboard
// image built from its own tag, and a dev build the newest release.
func TestKitImage(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })
	for v, want := range map[string]string{
		"0.7.0": "hevlayer/kit:0.7.0",
		"dev":   "hevlayer/kit:latest",
		"":      "hevlayer/kit:latest",
	} {
		Version = v
		if got := KitImage(); got != want {
			t.Errorf("Version %q: KitImage() = %q, want %q", v, got, want)
		}
	}
	Version = "0.7.0"
	if got := String(); got != "0.7.0" {
		t.Errorf("String() = %q, want 0.7.0", got)
	}
}
