package layerenv

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func clean(t *testing.T) {
	t.Helper()
	for _, name := range Names() {
		t.Setenv(name, "")
	}
	warned = sync.Map{}
}

func TestHEVLayerAliases(t *testing.T) {
	for name, aliases := range Aliases {
		for _, alias := range aliases {
			t.Run(alias, func(t *testing.T) {
				clean(t)
				t.Setenv(alias, "old-value")
				if got := Get(name); got != "old-value" {
					t.Fatalf("alias resolved to %q", got)
				}
				var out bytes.Buffer
				if err := Validate(&out); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.String(), alias+" is deprecated; use "+name) {
					t.Fatalf("warning: %s", &out)
				}
				if err := Validate(&out); err != nil {
					t.Fatal(err)
				}
				if strings.Count(out.String(), "warning:") != 1 {
					t.Fatalf("warning repeated: %s", &out)
				}
				t.Setenv(name, "new-value")
				if Get(name) != "new-value" {
					t.Fatal("canonical value did not win lookup")
				}
				err := Validate(&out)
				if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), alias) {
					t.Fatalf("conflict: %v", err)
				}
				if strings.Contains(err.Error(), "old-value") || strings.Contains(err.Error(), "new-value") {
					t.Fatal("conflict leaked values")
				}
				t.Setenv(alias, "new-value")
				if err := Validate(&out); err != nil {
					t.Fatalf("equal values conflict: %v", err)
				}
			})
		}
	}
}

func TestHEVLayerConflictingAliases(t *testing.T) {
	clean(t)
	t.Setenv("LAYER_API_KEY", "first-secret")
	t.Setenv("TPUF_API_KEY", "second-secret")
	err := Validate(&bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "LAYER_API_KEY") || !strings.Contains(err.Error(), "TPUF_API_KEY") {
		t.Fatalf("conflict: %v", err)
	}
}
