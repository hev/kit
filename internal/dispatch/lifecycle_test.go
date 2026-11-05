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
