//go:build !windows

package nodes

import (
	"os"
	"testing"
)

func assertPrivateJoinKey(t *testing.T, path string) {
	t.Helper()
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("private key mode must be 0600, stat=%v err=%v", st, err)
	}
}
