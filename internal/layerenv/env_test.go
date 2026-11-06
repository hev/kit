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
	warned = sync.Once{}
}

func TestHEVLayerAliases(t *testing.T) {
	for name, aliases := range Aliases {
		for _, alias := range aliases {
			t.Run(alias, func(t *testing.T) {
				clean(t)
				t.Setenv(alias, "old-value")
				if Get(name) != "old-value" {
					t.Fatal("fallback ignored")
				}
				var out bytes.Buffer
				WarnDeprecated(&out)
				if !strings.Contains(out.String(), alias+" is deprecated; use "+name) {
					t.Fatalf("warning: %s", &out)
				}
				t.Setenv(name, "new-value")
				WarnDeprecated(&out)
				if Get(name) != "new-value" {
					t.Fatal("canonical name did not win")
				}
				if strings.Count(out.String(), "warning:") != 1 {
					t.Fatal("warning repeated")
				}
				if strings.Contains(out.String(), "old-value") || strings.Contains(out.String(), "new-value") {
					t.Fatal("warning disclosed values")
				}
			})
		}
	}
}

func TestHEVLayerDifferentAliasesAreNonfatalFallbacks(t *testing.T) {
	clean(t)
	t.Setenv("LAYER_API_KEY", "gateway-key")
	t.Setenv("TURBOPUFFER_API_KEY", "other-key")
	t.Setenv("TPUF_API_KEY", "third-key")
	t.Setenv("LAYER_ENDPOINT", "https://gateway.example")
	var out bytes.Buffer
	WarnDeprecated(&out)
	WarnDeprecated(&out)
	if Get("HEVLAYER_API_KEY") != "gateway-key" {
		t.Fatal("fallback order changed")
	}
	if strings.Count(out.String(), "warning:") != 1 {
		t.Fatal("more than one deprecation warning")
	}
	t.Setenv("HEVLAYER_API_KEY", "canonical-key")
	if Get("HEVLAYER_API_KEY") != "canonical-key" {
		t.Fatal("canonical name did not win")
	}
}
