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
	Remote   *Remote   `json:"remote,omitempty"`
}
type Dispatch struct {
	Version          string `json:"version"`
	ProfileID        string `json:"profile_id"`
	DescriptorRef    string `json:"descriptor_ref"`
	DescriptorDigest string `json:"descriptor_digest"`
}

func canonical(p string) bool { return filepath.IsAbs(p) && filepath.Clean(p) == p }
func hash(b []byte) string    { d := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(d[:]) }
func digest(s string) bool {
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(s[7:])
	return e == nil
}
func private(p string, dir bool) error { return platformPrivate(p, dir) }
func pinned(p, d string, executable bool) error {
	if !digest(d) {
		return ErrHeld
	}
	f, e := openPinned(p, executable)
	if e != nil {
		return ErrHeld
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil || "sha256:"+hex.EncodeToString(h.Sum(nil)) != d {
		return ErrHeld
	}
	return nil
}
func readPrivate(path string) ([]byte, error) {
	f, e := openPrivate(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxFrame+1))
	if e != nil || len(b) > maxFrame {
		return nil, ErrHeld
	}
	return b, nil
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
	b, e := readPrivate(path)
	if e != nil || strict(b, &c) != nil || c.Version != Version || validateEndpoint(c.Socket) != nil || len(c.Profiles) == 0 || len(c.Profiles) > 32 {
		return Config{}, ErrHeld
	}
	if c.Remote != nil && c.Remote.verify() != nil {
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
	if e := platformRuntime(p.Runtime); e != nil {
		return e
	}
	var path string
	for _, value := range p.Runtime.Environment {
		if strings.HasPrefix(value, "PATH=") {
			path = strings.TrimPrefix(value, "PATH=")
		}
	}
	return verifyInterpreters(p, path)
}
