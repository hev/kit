package dispatch

import (
	"strings"
	"testing"
)

func TestHEVLayerOrderNamespaceOverridesAliases(t *testing.T) {
	env := namespaceEnvironment([]string{"LAYER_NAMESPACE=host", "HEVLAYER_NAMESPACE=host", "PATH=/bin"}, "order")
	if strings.Join(env, ",") != "PATH=/bin,HEVLAYER_NAMESPACE=order" {
		t.Fatalf("namespace environment: %v", env)
	}
}

func TestHEVLayerLaunchDoesNotInheritProviderKeys(t *testing.T) {
	for _, key := range []string{"TURBOPUFFER_API_KEY", "TPUF_API_KEY", "LAYER_API_KEY", "HEVLAYER_API_KEY"} {
		t.Setenv(key, "fixture")
	}
	m := &Meta{}
	m.Host = "h"
	resolveLaunch(m, Config{})
	names := "," + strings.Join(m.LaunchEnv, ",") + ","
	for _, key := range []string{"TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		if strings.Contains(names, ","+key+",") {
			t.Fatalf("provider credential inherited: %s", key)
		}
	}
	for _, key := range []string{"LAYER_API_KEY", "HEVLAYER_API_KEY"} {
		if !strings.Contains(names, ","+key+",") {
			t.Fatalf("gateway compatibility lost: %s", key)
		}
	}
}
