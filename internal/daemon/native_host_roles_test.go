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

func TestExplicitDaemonOptionsCannotBorrowAmbientProfile(t *testing.T) {
	t.Setenv("HEV_NATIVE_HOST_CONFIG", "/ambient/profile.json")
	t.Setenv("HEV_CAPTURE_ROLE", "required")
	if e := RunWithOptions(context.Background(), RunOptions{CaptureRole: "held"}); e == nil || e.Error() != "held capture requires explicit native host profile" {
		t.Fatal("explicit held options borrowed an ambient profile", e)
	}
	if e := RunWithOptions(context.Background(), RunOptions{CaptureRole: "optional"}); e == nil || e.Error() != "invalid capture role" {
		t.Fatal("unknown explicit role accepted", e)
	}
}
