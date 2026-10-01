package fixture

import (
	"bytes"
	"github.com/hev/kit/internal/redact"
	"strings"
	"testing"
)

func TestEveryTextRuleFixture(t *testing.T) {
	samples, err := Samples()
	if err != nil {
		t.Fatal(err)
	}
	s, err := redact.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		t.Run(sample.Rule, func(t *testing.T) {
			clean, counts := s.Text(sample.Text)
			if len(counts) == 0 || strings.Contains(clean, sample.Secret) {
				t.Fatal("original survives")
			}
			again, c := s.Text(clean)
			if again != clean || len(c) != 0 {
				t.Fatal("marker not idempotent")
			}
		})
	}
	t.Logf("%d synthetic rule/envelope samples", len(samples))
}
