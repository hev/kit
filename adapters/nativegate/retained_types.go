package nativegate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	n "github.com/hev/kit/pkg/nativejob"
)

const SupervisorVersion = "native-supervisor/v1"
const SupervisorReadyVersion = "native-supervisor-ready/v1"

type RetainedConfig struct {
	Version                string `json:"version"`
	Launcher               string `json:"launcher"`
	LauncherDigest         string `json:"launcher_digest"`
	Supervisor             string `json:"supervisor"`
	SupervisorDigest       string `json:"supervisor_digest"`
	StateRoot              string `json:"state_root"`
	WorkloadTimeoutSeconds int    `json:"workload_timeout_seconds"`
}
type NativeSupervisorDescriptor struct {
	Version                string       `json:"version"`
	ConfigRef              string       `json:"config_ref"`
	ConfigDigest           string       `json:"config_digest"`
	SupervisorRef          string       `json:"supervisor_ref"`
	SupervisorDigest       string       `json:"supervisor_digest"`
	StartCall              ReceiverCall `json:"start_call"`
	WorkloadTimeoutSeconds int          `json:"workload_timeout_seconds"`
	DescriptorDigest       string       `json:"descriptor_digest,omitempty"`
}
type NativeSupervisorReady struct {
	Version          string               `json:"version"`
	DescriptorDigest string               `json:"descriptor_digest"`
	Observation      n.StartedObservation `json:"observation"`
}

func DescriptorDigest(d NativeSupervisorDescriptor) string {
	d.DescriptorDigest = ""
	return n.Digest(d)
}
func validFileDigest(s string) bool {
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(s[7:])
	return err == nil && strings.ToLower(s) == s
}
func (r RetainedConfig) valid() bool {
	return r.Version == SupervisorVersion && canonicalPath(r.Launcher) && canonicalPath(r.Supervisor) && canonicalPath(r.StateRoot) && validFileDigest(r.LauncherDigest) && validFileDigest(r.SupervisorDigest) && r.WorkloadTimeoutSeconds > 0 && r.WorkloadTimeoutSeconds <= 86400
}
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func pinnedExecutable(path, digest string) error {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 || st.Mode().Perm()&0111 == 0 {
		return ErrReceiverCall
	}
	got, err := fileDigest(path)
	if err != nil || got != digest {
		return ErrReceiverCall
	}
	return nil
}
func privateDirectory(path string) error {
	st, err := os.Lstat(path)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 || !ownedByCurrentUser(st) {
		return ErrReceiverCall
	}
	return nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func privateJSON(path string, out any) error {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || !ownedByCurrentUser(st) {
		return ErrReceiverCall
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = decodeExact(f, out)
	return err
}

// exclusiveJSON never truncates or replaces historical/partial evidence.
func exclusiveJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil || len(data) > 64<<10 {
		return ErrReceiverCall
	}
	// Publish complete bytes exclusively: readers never decode an in-progress
	// readiness payload. A surviving temporary file is never executable intent.
	f, err := os.CreateTemp(filepath.Dir(path), ".native-publish-")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Link(temp, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func canonicalStart(c ReceiverCall) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return ErrReceiverCall
	}
	got, err := DecodeReceiverCall(bytes.NewReader(raw))
	if err != nil || got.Operation != "start" || got.Binding == nil || got.Claim == nil || got.Binding.RequestDigest == "" {
		return ErrReceiverCall
	}
	claim := got.Claim
	b := got.Binding
	if claim.Lease == "" || claim.Holder == "" || claim.LeaseFence < 1 || claim.ActionID == "" || claim.Claimant != claim.Holder || claim.Attempt < 1 || claim.ClaimFence != claim.LeaseFence || b.OrderUID == "" || b.Generation < 1 || b.ScopeDigest == "" {
		return ErrReceiverCall
	}
	return nil
}

// UnmarshalJSON preserves raw start-call member presence, including unused null
// fields which ordinary struct round trips would normalize away.
func (d *NativeSupervisorDescriptor) UnmarshalJSON(data []byte) error {
	type plain NativeSupervisorDescriptor
	var p plain
	if _, err := decodeExact(bytes.NewReader(data), &p); err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(data, &members) != nil {
		return ErrReceiverCall
	}
	call, err := DecodeReceiverCall(bytes.NewReader(members["start_call"]))
	if err != nil || call.Operation != "start" {
		return ErrReceiverCall
	}
	p.StartCall = call
	*d = NativeSupervisorDescriptor(p)
	return nil
}
