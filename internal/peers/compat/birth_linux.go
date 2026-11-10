//go:build linux

package compat

import "golang.org/x/sys/unix"

// birthtime is os.stat(path).st_birthtime; Linux reads it through statx.
func birthtime(path string) (float64, bool) {
	var st unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_BTIME, &st); err != nil {
		return 0, false
	}
	if st.Mask&unix.STATX_BTIME == 0 {
		return float64(st.Mtime.Sec) + float64(st.Mtime.Nsec)/1e9, true
	}
	return float64(st.Btime.Sec) + float64(st.Btime.Nsec)/1e9, true
}
