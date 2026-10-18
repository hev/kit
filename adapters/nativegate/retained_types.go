package nativegate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
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
func hashHandle(f *os.File) (string, error) {
	h := sha256.New()
	if _, e := io.Copy(h, f); e != nil {
		return "", e
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func fileDigest(path string) (string, error) {
	f, e := openPinned(path, false)
	if e != nil {
		return "", e
	}
	defer f.Close()
	return hashHandle(f)
}
func pinnedExecutable(path, digest string) error {
	f, e := openPinned(path, true)
	if e != nil {
		return e
	}
	defer f.Close()
	got, e := hashHandle(f)
	if e != nil || got != digest {
		return ErrReceiverCall
	}
	return nil
}
func privateDirectory(path string) error { return platformPrivateDirectory(path) }
func syncDirectory(path string) error    { return platformSyncDirectory(path) }
func privateJSON(path string, out any) error {
	f, e := openPrivate(path)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = decodeExact(f, out)
	return e
}

// exclusiveJSON never truncates or replaces historical/partial evidence.
func exclusiveJSON(path string, v any) error {
	data, e := json.Marshal(v)
	if e != nil || len(data) > 64<<10 {
		return ErrReceiverCall
	}
	return platformPublishExclusive(path, data)
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
