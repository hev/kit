package index

import (
	"github.com/hev/kit/internal/trace"
	"testing"
)

func TestPiMigrationScope(t *testing.T) {
	root, harness := sourceScope(&trace.PiSource{Root: "/synthetic/pi"})
	if root != "/synthetic/pi" || harness != "pi" {
		t.Fatalf("%q %q", root, harness)
	}
}
