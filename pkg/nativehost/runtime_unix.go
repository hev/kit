//go:build linux || darwin

package nativehost

import (
	"os"
	"strings"
)

func platformRuntime(runtime Runtime) error {
	p := Profile{Runtime: runtime}
	if !canonical(p.Runtime.Directory) {
		return ErrHeld
	}
	st, e := os.Lstat(p.Runtime.Directory)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0022 != 0 {
		return ErrHeld
	}
	allowed := map[string]bool{"HOME": true, "PATH": true, "LANG": true, "LC_ALL": true, "TMPDIR": true, "NODE_EXTRA_CA_CERTS": true, "SSL_CERT_FILE": true}
	seen := map[string]string{}
	for _, v := range p.Runtime.Environment {
		k, value, ok := strings.Cut(v, "=")
		if !ok || !allowed[k] || seen[k] != "" || value == "" || strings.ContainsRune(value, 0) {
			return ErrHeld
		}
		seen[k] = value
	}
	if !canonical(seen["HOME"]) || seen["PATH"] == "" {
		return ErrHeld
	}
	for _, d := range strings.Split(seen["PATH"], string(os.PathListSeparator)) {
		if !canonical(d) {
			return ErrHeld
		}
		s, e := os.Stat(d)
		if e != nil || !s.IsDir() || s.Mode().Perm()&0022 != 0 {
			return ErrHeld
		}
	}
	pins := map[string]bool{}
	for _, f := range p.Runtime.Files {
		if pins[f.Path] || pinned(f.Path, f.Digest, false) != nil {
			return ErrHeld
		}
		pins[f.Path] = true
	}
	for _, k := range []string{"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE"} {
		if seen[k] != "" && !pins[seen[k]] {
			return ErrHeld
		}
	}
	return nil
}
