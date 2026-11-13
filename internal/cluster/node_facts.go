package cluster

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"time"
)

const FactsRefresh = time.Minute
const FactsStale = 3 * FactsRefresh
const maxNodeFacts = 4096

type DispatchFacts struct {
	At      time.Time `json:"at"`
	Cap     uint32    `json:"cap"`
	Running uint32    `json:"running"`
	Free    uint32    `json:"free"`
	Queue   uint32    `json:"queue"`
}
type SessionCount struct {
	Working uint32 `json:"working"`
	Idle    uint32 `json:"idle"`
}
type SessionFacts struct {
	At      time.Time               `json:"at"`
	Harness map[string]SessionCount `json:"harness"`
	Role    map[string]SessionCount `json:"role"`
}
type BuildFacts struct {
	At        time.Time `json:"at"`
	Commit    string    `json:"commit"`
	Staged    bool      `json:"staged"`
	LastCheck time.Time `json:"last_check"`
	Outcome   string    `json:"outcome"`
}
type CredentialFact struct {
	At    time.Time `json:"at"`
	State string    `json:"state"`
}
type CredentialFacts struct {
	At    time.Time                 `json:"at"`
	Items map[string]CredentialFact `json:"items"`
}
type CaptureFacts struct {
	At         time.Time `json:"at"`
	Running    bool      `json:"running"`
	LastUpload time.Time `json:"last_upload"`
	Backlog    uint32    `json:"backlog"`
	Error      string    `json:"error"`
}

// NodeFacts contains no arbitrary user strings. Optional sections are genuinely absent.
type NodeFacts struct {
	Version     int              `json:"v"`
	At          time.Time        `json:"at"`
	Dispatch    *DispatchFacts   `json:"dispatch,omitempty"`
	Sessions    *SessionFacts    `json:"sessions,omitempty"`
	Build       *BuildFacts      `json:"build,omitempty"`
	Credentials *CredentialFacts `json:"credentials,omitempty"`
	Capture     *CaptureFacts    `json:"capture,omitempty"`
}

// Reject unknown fields, including attempts to smuggle free text into the schema.
func (f *NodeFacts) UnmarshalJSON(b []byte) error {
	if len(b) > maxNodeFacts {
		return ErrBound
	}
	type plain NodeFacts
	var next plain
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&next); err != nil {
		return ErrBound
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return ErrBound
	}
	*f = NodeFacts(next)
	return nil
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{7,12}$`)

func label(s string, allowed ...string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}
func validNodeFacts(node string, f NodeFacts, now time.Time) bool {
	validTime := func(t time.Time, optional bool) bool {
		return optional && t.IsZero() || !t.IsZero() && t.Year() >= 1970 && t.Year() <= 9999 && !t.After(now.Add(usageSkew)) && !t.After(f.At)
	}
	if !identifier(node) || f.Version != 1 || !validTime(f.At, false) {
		return false
	}
	if d := f.Dispatch; d != nil {
		if !validTime(d.At, false) || d.Free != uint32(max(0, int64(d.Cap)-int64(d.Running))) {
			return false
		}
	}
	if s := f.Sessions; s != nil {
		if !validTime(s.At, false) || len(s.Harness) > 5 || len(s.Role) > 2 {
			return false
		}
		for k := range s.Harness {
			if !label(k, "claude", "codex", "pi", "opencode", "other") {
				return false
			}
		}
		for k := range s.Role {
			if !label(k, "foh", "boh") {
				return false
			}
		}
	}
	if b := f.Build; b != nil {
		if !validTime(b.At, false) || !validTime(b.LastCheck, true) || b.Commit != "" && !commitPattern.MatchString(b.Commit) || !label(b.Outcome, "ok", "failing", "unknown") {
			return false
		}
	}
	if c := f.Credentials; c != nil {
		if !validTime(c.At, false) || len(c.Items) > 7 {
			return false
		}
		for k, v := range c.Items {
			if !label(k, "op", "azkv", "layer", "claude", "codex", "pi", "opencode") || !label(v.State, "ok", "failing", "unknown") || !validTime(v.At, v.State == "unknown") {
				return false
			}
		}
	}
	if c := f.Capture; c != nil {
		if !validTime(c.At, false) || !validTime(c.LastUpload, true) || !label(c.Error, "", "auth", "rate-limit", "network", "storage", "other") {
			return false
		}
	}
	raw, err := json.Marshal(f)
	return err == nil && len(raw) <= maxNodeFacts
}
func copyNodeFacts(f NodeFacts) NodeFacts {
	b, _ := json.Marshal(f)
	var out NodeFacts
	_ = json.Unmarshal(b, &out)
	return out
}
func (c *Counters) SetNodeFacts(f NodeFacts) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ready || !validNodeFacts(c.node, f, c.now()) {
		return ErrBound
	}
	next := cloneState(c.state)
	prune(&next, c.now())
	if next.Facts == nil {
		next.Facts = map[string]NodeFacts{}
	}
	if !f.At.After(next.Facts[c.node].At) {
		return nil
	}
	next.Facts[c.node] = copyNodeFacts(f)
	if len(next.Facts) > 64 || !stateWithinBounds(next) {
		return ErrBound
	}
	c.state = next
	c.emit(counterState{Facts: map[string]NodeFacts{c.node: f}})
	return nil
}
func cloneState(s counterState) counterState {
	b, _ := json.Marshal(s)
	var out counterState
	_ = json.Unmarshal(b, &out)
	return out
}
func (c *Counters) NodeFacts() map[string]NodeFacts {
	c.mu.Lock()
	defer c.mu.Unlock()
	prune(&c.state, c.now())
	out := map[string]NodeFacts{}
	for n, f := range c.state.Facts {
		out[n] = copyNodeFacts(f)
	}
	return out
}

// Ages measures each section independently; unknown observation times use -1.
func (f NodeFacts) Ages(now time.Time) map[string]int64 {
	out := map[string]int64{}
	add := func(k string, t time.Time) {
		if t.IsZero() {
			out[k] = -1
		} else {
			out[k] = max(0, int64(now.Sub(t).Seconds()))
		}
	}
	add("record", f.At)
	if f.Dispatch != nil {
		add("dispatch", f.Dispatch.At)
	}
	if f.Sessions != nil {
		add("sessions", f.Sessions.At)
	}
	if f.Build != nil {
		add("build", f.Build.At)
	}
	if f.Credentials != nil {
		add("credentials", f.Credentials.At)
		for k, v := range f.Credentials.Items {
			add("credential."+k, v.At)
		}
	}
	if f.Capture != nil {
		add("capture", f.Capture.At)
	}
	return out
}

func (f NodeFacts) Stale(now time.Time) []string {
	out := []string{}
	for _, k := range []string{"dispatch", "sessions", "build", "credentials", "capture"} {
		if a, ok := f.Ages(now)[k]; ok && a > int64(FactsStale.Seconds()) {
			out = append(out, k)
		}
	}
	return out
}
