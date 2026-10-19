//go:build !windows

package nativehost

import n "github.com/hev/kit/pkg/nativejob"

func platformEnterService(configPath string, hostBirth n.Birth) (func() error, error) {
	return func() error { return nil }, nil
}
func platformObserveService(expectedBirth n.Birth, configPath string) (ServiceObservation, error) {
	return ServiceObservation{}, ErrHeld
}
