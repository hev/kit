// Package nativehost hosts fixed native receivers inside the managed capture
// daemon. It is transport, not admission, a scheduler, or a result journal.
package nativehost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	n "github.com/hev/kit/pkg/nativejob"
)

const Version = "native-host/v1"
const DispatchVersion = "hevd-native-dispatch/v1"
const RegistrationVersion = "native-host-registration/v1"

var ErrHeld = errors.New("native host held; inspect original descriptor and journal; do not replay")

const maxFrame = 64 << 10

type FilePin struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}
type Runtime struct {
	Directory   string    `json:"directory"`
	Environment []string  `json:"environment"`
	Files       []FilePin `json:"files"`
}
type Profile struct {
	ID                  string       `json:"id"`
	ConfigRef           string       `json:"config_ref"`
	ConfigDigest        string       `json:"config_digest"`
	SupervisorRef       string       `json:"supervisor_ref"`
	SupervisorDigest    string       `json:"supervisor_digest"`
	StateRoot           string       `json:"state_root"`
	Enrollment          n.Enrollment `json:"enrollment"`
	NodeUID             string       `json:"node_uid"`
	Incarnation         string       `json:"incarnation"`
	AdmissionGeneration uint64       `json:"admission_generation"`
	Runtime             Runtime      `json:"runtime"`
}
type Config struct {
	Version  string    `json:"version"`
	Socket   string    `json:"socket"`
	Profiles []Profile `json:"profiles"`
}
type Dispatch struct {
	Version          string `json:"version"`
	ProfileID        string `json:"profile_id"`
	DescriptorRef    string `json:"descriptor_ref"`
	DescriptorDigest string `json:"descriptor_digest"`
}

func canonical(p string) bool { return filepath.IsAbs(p) && filepath.Clean(p) == p }
func hash(b []byte) string    { d := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(d[:]) }
func fileHash(p string) (string, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func digest(s string) bool {
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(s[7:])
	return e == nil
}
func private(p string, dir bool) error {
	if !canonical(p) {
		return ErrHeld
	}
	s, e := os.Lstat(p)
	if e != nil || !owner(s) || s.Mode().Perm()&0077 != 0 {
		return ErrHeld
	}
	if dir {
		if !s.IsDir() {
			return ErrHeld
		}
	} else if !s.Mode().IsRegular() {
		return ErrHeld
	}
	return nil
}
func pinned(p, d string, executable bool) error {
	if !canonical(p) || !digest(d) {
		return ErrHeld
	}
	s, e := os.Lstat(p)
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm()&0022 != 0 {
		return ErrHeld
	}
	if executable && s.Mode().Perm()&0111 == 0 {
		return ErrHeld
	}
	got, e := fileHash(p)
	if e != nil || got != d {
		return ErrHeld
	}
	return nil
}

// strict rejects duplicate and aliased members recursively before normal decode.
func strict(data []byte, out any) error {
	if len(data) > maxFrame {
		return ErrHeld
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if e := unique(d); e != nil {
		return ErrHeld
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrHeld
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return ErrHeld
	}
	// JSON field-name matching is case insensitive; compare canonical names with
	// the decoded form recursively to reject aliases even in nested structs.
	canonicalBytes, e := json.Marshal(out)
	if e != nil {
		return ErrHeld
	}
	var a, b any
	if json.Unmarshal(data, &a) != nil || json.Unmarshal(canonicalBytes, &b) != nil {
		return ErrHeld
	}
	if !sameKeys(a, b) {
		return ErrHeld
	}
	return nil
}
func sameKeys(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !sameKeys(v, w) {
				return false
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !sameKeys(x[i], y[i]) {
				return false
			}
		}
	}
	return true
}
func unique(d *json.Decoder) error {
	t, e := d.Token()
	if e != nil {
		return e
	}
	switch t {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return e
			}
			s, ok := k.(string)
			if !ok || seen[s] {
				return ErrHeld
			}
			seen[s] = true
			if e = unique(d); e != nil {
				return e
			}
		}
		_, e = d.Token()
		return e
	case json.Delim('['):
		for d.More() {
			if e = unique(d); e != nil {
				return e
			}
		}
		_, e = d.Token()
		return e
	}
	return nil
}

// VerifyConfig performs read-only static validation. It never opens a socket,
// starts a process, or creates journals or database connections.
func VerifyConfig(path string) (Config, error) {
	var c Config
	if private(filepath.Dir(path), true) != nil || private(path, false) != nil {
		return c, ErrHeld
	}
	b, e := os.ReadFile(path)
	if e != nil || strict(b, &c) != nil || c.Version != Version || !canonical(c.Socket) || private(filepath.Dir(c.Socket), true) != nil || len(c.Profiles) == 0 || len(c.Profiles) > 32 {
		return Config{}, ErrHeld
	}
	seen := map[string]bool{}
	configs := map[string]bool{}
	states := map[string]bool{}
	for _, p := range c.Profiles {
		if seen[p.ID] || configs[p.ConfigRef] || states[p.StateRoot] || p.verify() != nil {
			return Config{}, ErrHeld
		}
		seen[p.ID] = true
		configs[p.ConfigRef] = true
		states[p.StateRoot] = true
	}
	return c, nil
}
func (p Profile) verify() error {
	if p.ID == "" || p.Enrollment.HostID == "" || p.Enrollment.ExecutorID == "" || p.NodeUID == "" || p.Incarnation == "" || p.AdmissionGeneration == 0 || private(p.StateRoot, true) != nil || private(p.ConfigRef, false) != nil || pinned(p.ConfigRef, p.ConfigDigest, false) != nil || pinned(p.SupervisorRef, p.SupervisorDigest, true) != nil {
		return ErrHeld
	}
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
