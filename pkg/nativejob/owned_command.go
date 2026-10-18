package nativejob

import "errors"

// ErrOwnedCommandUnknown prevents a terminal result/ACK when the platform's
// owned process or required tree boundary could not be established.
var ErrOwnedCommandUnknown = errors.New("owned native workload outcome unknown; retain original uncertainty")

// ownedCommand is internal lifecycle ownership, not a public argv/PID service.
// A known exit code may be returned only after the platform's required terminal
// boundary. Unix preserves the original root Wait contract. Windows additionally
// requires the per-workload owned Job Object to be empty before known completion.
type ownedCommand interface {
	Birth() (Birth, error)
	Wait() (int, error)
	Kill() error
}
