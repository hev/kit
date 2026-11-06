//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd

package maintenance

import (
	"context"
	"errors"
	"os"
)

func lockStore(context.Context, string) (func(), error) {
	return nil, errors.New("UnsupportedCapability: durable maintenance locking unavailable on this platform")
}

// Until no-follow, nonblocking regular-file handles are implemented on this
// platform, refuse reads rather than risk following reparse points or pipes.
func openRegularSnapshot(string) (*os.File, error) {
	return nil, errors.New("UnsupportedCapability: safe maintenance snapshot handles unavailable on this platform")
}
