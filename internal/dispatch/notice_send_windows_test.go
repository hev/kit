package dispatch

import "testing"

// The Windows CI selector runs Windows-prefixed tests only.
func TestWindowsNoticeSendHoldsNoLock(t *testing.T) { TestNoticeSendHoldsNoLock(t) }
