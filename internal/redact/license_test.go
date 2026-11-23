package redact

import (
	"strings"
	"testing"
)

func TestPublicTokenRedactionKeepsPasetoProse(t *testing.T) {
	s, err := New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	const token = "v4.public.payload-123.footer_456"
	const prose = "Kit verifies PASETO v4.public."
	got, counts := s.Text(prose + "\n" + token)
	if !strings.HasPrefix(got, prose) || strings.Contains(got, token) || counts["paseto-v4-public"] != 1 {
		t.Fatal("PASETO scrubbing must preserve prose and mask the token")
	}
}
