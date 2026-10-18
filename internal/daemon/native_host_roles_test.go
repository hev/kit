package daemon

import (
	"context"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/platformops"
	"testing"
)

func TestHeldCaptureRequiresSpecificUnsupportedCapability(t *testing.T) {
	for _, e := range []error{nil, errors.New("credential failure"), errors.New("config failure"), errors.New("network failure"), errors.New("UnsupportedCapability: text alone")} {
		if requireHeldCapture(e) == nil {
			t.Fatalf("accepted held capture for %v", e)
		}
	}
	if e := requireHeldCapture(fmt.Errorf("wrapped: %w", platformops.ErrTraceWritesUnsupported)); e != nil {
		t.Fatal(e)
	}
}
func TestHeldCaptureRequiresExplicitNativeProfile(t *testing.T) {
	t.Setenv("HEV_CAPTURE_ROLE", "held")
	t.Setenv("HEV_NATIVE_HOST_CONFIG", "")
	if e := Run(context.Background()); e == nil {
		t.Fatal("held without native profile accepted")
	}
}
func TestUnknownCaptureRoleRefusesBeforeCapture(t *testing.T) {
	t.Setenv("HEV_CAPTURE_ROLE", "optional")
	t.Setenv("HEV_NATIVE_HOST_CONFIG", "")
	if e := Run(context.Background()); e == nil {
		t.Fatal("unknown role accepted")
	}
}
