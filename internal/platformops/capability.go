package platformops

import "errors"

// ErrTraceWritesUnsupported identifies the native trace-write capability gap.
// Authentication, configuration and network errors must never use this sentinel.
var ErrTraceWritesUnsupported = errors.New("UnsupportedCapability: Windows trace writes and daemon lifecycle require native locking, ACL and durability acceptance")
