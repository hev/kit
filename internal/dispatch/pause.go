package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
)

// Host pause shares the admission lock with launches. Existing runs retain
// their lifecycle controls; pausing never interrupts a process.
func controlHostPause(action string) error {
	if err := os.MkdirAll(Root(), 0700); err != nil {
		return err
	}
	unlock, err := lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(Root(), ".paused")
	if action == "pause" {
		return os.WriteFile(path, []byte("paused\n"), 0600)
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func checkHostPause() error {
	_, err := os.Stat(filepath.Join(Root(), ".paused"))
	if err == nil {
		return fmt.Errorf("host dispatch paused; use hev dispatch resume --host HOST")
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("host dispatch pause state: %w", err)
}
