//go:build !windows

package skillsources

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestSyncAcrossUmasks(t *testing.T) {
	original := syscall.Umask(0002)
	defer syscall.Umask(original)
	for _, gitSource := range []bool{false, true} {
		r := t.TempDir()
		if gitSource {
			r = repository(t)
		}
		put(t, filepath.Join(r, "example", "SKILL.md"), "---\nharnesses: all\n---\nexample")
		put(t, filepath.Join(r, "example", "bin", "hello"), "hello")
		if gitSource {
			commit(t, r)
		}
		h := t.TempDir()
		c := Config{Sources: []Source{{Path: r}}}
		for _, mask := range []int{0002, 0022, 0002, 0022} {
			syscall.Umask(mask)
			apply(t, h, c, true)
			apply(t, h, c, false)
		}
	}
}
