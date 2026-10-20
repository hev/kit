//go:build windows

package nativejob

import (
	"errors"
	"github.com/hev/kit/internal/winplatform"
)

// This permits the real checked store path only on the native-proved build.
// Every root/DB/WAL/SHM still passes the protected-owner, no-reparse, NTFS and
// durability hooks. It is not enrollment, execution permission or tree proof.
func journalPlatformPreflight() error {
	if !winplatform.ExecutionBuild() {
		return errors.New("Windows native journal not enabled for this kernel/architecture")
	}
	return nil
}
