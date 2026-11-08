//go:build !linux && !darwin

package compat

import "os"

func birthtime(path string) (float64, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return float64(st.ModTime().UnixNano()) / 1e9, true
}
