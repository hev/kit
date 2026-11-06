package redact

import (
	"bytes"
	"strings"
	"testing"
)

func TestHEVLayerKeyRedaction(t *testing.T) {
	s, err := New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret := "J7u9Bq2Lx5Nw8Rf0Vh3Cs6Ma1Zk4Dp9T"
	for _, name := range []string{"HEVLAYER_API_KEY", "HEVLAYER_PROVIDER_KEY", "LAYER_API_KEY", "TURBOPUFFER_API_KEY", "TPUF_API_KEY"} {
		text, counts := s.Text("export " + name + "=" + secret)
		if strings.Contains(text, secret) || len(counts) == 0 {
			t.Fatalf("%s was not scrubbed", name)
		}
	}
}
