//go:build !darwin

package nativehost

import n "github.com/hev/kit/pkg/nativejob"

func observeResourceCoalition(n.Birth, uint64) (CoalitionObservation, error) {
	return CoalitionObservation{}, ErrHeld
}
