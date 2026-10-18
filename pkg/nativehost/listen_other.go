//go:build !linux && !darwin

package nativehost

import "context"

func RunConfig(context.Context, string) error { return ErrHeld }
