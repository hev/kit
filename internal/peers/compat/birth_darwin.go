//go:build darwin

package compat

import "syscall"

func birthtime(path string) (float64, bool) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, false
	}
	return float64(st.Birthtimespec.Sec) + float64(st.Birthtimespec.Nsec)/1e9, true
}
