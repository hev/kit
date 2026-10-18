package nativehost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	n "github.com/hev/kit/pkg/nativejob"
)

type Registration struct {
	Version             string       `json:"version"`
	Enrollment          n.Enrollment `json:"enrollment"`
	NodeUID             string       `json:"node_uid"`
	Incarnation         string       `json:"incarnation"`
	AdmissionGeneration uint64       `json:"admission_generation"`
	ObservedAt          time.Time    `json:"observed_at"`
}

// RegistrationRequest asks for a local projection, never a new enrollment.
type RegistrationRequest struct {
	Version             string       `json:"version"`
	Enrollment          n.Enrollment `json:"enrollment"`
	NodeUID             string       `json:"node_uid"`
	Incarnation         string       `json:"incarnation"`
	AdmissionGeneration uint64       `json:"admission_generation"`
	DescriptorRef       string       `json:"descriptor_ref,omitempty"`
	DescriptorDigest    string       `json:"descriptor_digest,omitempty"`
}
type RegistrationReceipt struct {
	Version       string       `json:"version"`
	ProfileID     string       `json:"profile_id"`
	ProfileDigest string       `json:"profile_digest"`
	HostBirth     n.Birth      `json:"host_birth"`
	Protocol      string       `json:"protocol"`
	Registration  Registration `json:"registration"`
}
type Host interface {
	Dispatch(context.Context, Dispatch) error
}
type host struct {
	profiles   map[string]Profile
	birth      n.Birth
	mu         sync.Mutex
	active     int
	registered map[string]bool
}

type limitedOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > maxFrame-b.Len() {
		b.overflow = true
		return 0, ErrHeld
	}
	return b.Buffer.Write(p)
}

// Do not expose bytes.Buffer.ReadFrom: exec.Cmd would bypass Write's limit.
type writerOnly struct{ io.Writer }

func command(ctx context.Context, p Profile, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, p.SupervisorRef, args...)
	c.Dir = p.Runtime.Directory
	c.Env = append([]string(nil), p.Runtime.Environment...)
	c.WaitDelay = 2 * time.Second
	return c
}
func inspectRegistration(ctx context.Context, p Profile, call Dispatch) (Registration, error) {
	var r Registration
	if p.verify() != nil {
		return r, ErrHeld
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := command(ctx, p, "--config", p.ConfigRef, "--register-host")
	input, err := json.Marshal(RegistrationRequest{RegistrationVersion, p.Enrollment, p.NodeUID, p.Incarnation, p.AdmissionGeneration, call.DescriptorRef, call.DescriptorDigest})
	if err != nil {
		return r, ErrHeld
	}
	c.Stdin = bytes.NewReader(input)
	var output limitedOutput
	c.Stdout = writerOnly{&output}
	c.Stderr = io.Discard
	if c.Run() != nil || output.overflow || strict(output.Bytes(), &r) != nil || r.Version != RegistrationVersion || r.Enrollment != p.Enrollment || r.NodeUID != p.NodeUID || r.Incarnation != p.Incarnation || r.AdmissionGeneration != p.AdmissionGeneration || r.ObservedAt.IsZero() || r.ObservedAt.Before(time.Now().Add(-30*time.Second)) || r.ObservedAt.After(time.Now().Add(5*time.Second)) {
		return Registration{}, ErrHeld
	}
	return r, nil
}
func syncDir(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func exclusive(path string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(append(b, '\n'))
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return syncDir(filepath.Dir(path))
}

type descriptor struct {
	Version                string          `json:"version"`
	ConfigRef              string          `json:"config_ref"`
	ConfigDigest           string          `json:"config_digest"`
	SupervisorRef          string          `json:"supervisor_ref"`
	SupervisorDigest       string          `json:"supervisor_digest"`
	StartCall              json.RawMessage `json:"start_call"`
	WorkloadTimeoutSeconds int             `json:"workload_timeout_seconds"`
	DescriptorDigest       string          `json:"descriptor_digest,omitempty"`
}

func validateDescriptor(p Profile, call Dispatch) error {
	if call.Version != DispatchVersion || call.ProfileID != p.ID || !digest(call.DescriptorDigest) || private(call.DescriptorRef, false) != nil || private(filepath.Dir(call.DescriptorRef), true) != nil || filepath.Dir(filepath.Dir(call.DescriptorRef)) != p.StateRoot || filepath.Base(call.DescriptorRef) != "descriptor.json" {
		return ErrHeld
	}
	b, e := os.ReadFile(call.DescriptorRef)
	var d descriptor
	if e != nil || strict(b, &d) != nil || d.Version != "native-supervisor/v1" || d.ConfigRef != p.ConfigRef || d.ConfigDigest != p.ConfigDigest || d.SupervisorRef != p.SupervisorRef || d.SupervisorDigest != p.SupervisorDigest || d.DescriptorDigest != call.DescriptorDigest || d.WorkloadTimeoutSeconds < 1 || d.WorkloadTimeoutSeconds > 86400 {
		return ErrHeld
	}
	d.DescriptorDigest = ""
	if n.Digest(d) != call.DescriptorDigest {
		return ErrHeld
	}
	var c struct {
		Version   string          `json:"version"`
		Operation string          `json:"operation"`
		JobID     string          `json:"job_id"`
		Claim     json.RawMessage `json:"claim"`
		Binding   json.RawMessage `json:"binding"`
	}
	if strict(d.StartCall, &c) != nil || c.Version != "native-receiver/v1" || c.Operation != "start" || c.JobID == "" || len(c.Claim) == 0 || string(c.Claim) == "null" || len(c.Binding) == 0 || string(c.Binding) == "null" || filepath.Base(filepath.Dir(call.DescriptorRef)) != strings.TrimPrefix(n.Digest(c.JobID), "sha256:") {
		return ErrHeld
	}
	return nil
}
func (h *host) Dispatch(ctx context.Context, call Dispatch) error {
	if ctx.Err() != nil {
		return ErrHeld
	}
	p, ok := h.profiles[call.ProfileID]
	if !ok || p.verify() != nil || validateDescriptor(p, call) != nil {
		return ErrHeld
	}
	h.mu.Lock()
	if !h.registered[p.ID] || h.active >= 16 {
		h.mu.Unlock()
		return ErrHeld
	}
	h.active++
	h.mu.Unlock()
	started := false
	defer func() {
		if !started {
			h.mu.Lock()
			h.active--
			h.mu.Unlock()
		}
	}()
	r, e := inspectRegistration(ctx, p, call)
	if e != nil {
		return ErrHeld
	}
	// One immutable registration observation per original descriptor, not a
	// capacity permit. Final native/maintenance/PG checks still happen in worker.
	dir := filepath.Dir(call.DescriptorRef)
	receipt := RegistrationReceipt{Version: RegistrationVersion, ProfileID: p.ID, ProfileDigest: n.Digest(p), HostBirth: h.birth, Protocol: DispatchVersion, Registration: r}
	if exclusive(filepath.Join(dir, "host-registration.json"), receipt) != nil {
		return ErrHeld
	}
	intent := struct {
		Version   string   `json:"version"`
		Dispatch  Dispatch `json:"dispatch"`
		HostBirth n.Birth  `json:"host_birth"`
	}{DispatchVersion, call, h.birth}
	if exclusive(filepath.Join(dir, "host-intent.json"), intent) != nil {
		return ErrHeld
	}
	// Runtime pins checked again immediately before this FIXED receiver spawn.
	// This is not the workload launch permission: the worker holds the actual
	// final DB/native/business/maintenance guards through workload cmd.Start.
	if p.verify() != nil {
		return ErrHeld
	}
	c := exec.Command(p.SupervisorRef, "--config", p.ConfigRef, "--supervise", call.DescriptorRef)
	c.Dir = p.Runtime.Directory
	c.Env = append([]string(nil), p.Runtime.Environment...)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	if c.Start() != nil {
		return ErrHeld
	}
	started = true
	go func() { _ = c.Wait(); h.mu.Lock(); h.active--; h.mu.Unlock() }()
	return nil
}

// Run serves ONLY owner-authenticated platform connections, even when the listener
// is injected. Caller cancellation never kills an accepted receiver process.
func Run(ctx context.Context, profiles []Profile, listener net.Listener) error {
	if !supported() || len(profiles) == 0 || len(profiles) > 32 {
		return ErrHeld
	}
	if verifyListener(listener) != nil {
		return ErrHeld
	}
	birth, e := n.ProcessBirth(os.Getpid())
	if e != nil {
		return e
	}
	h := &host{profiles: map[string]Profile{}, birth: birth, registered: map[string]bool{}}
	for _, p := range profiles {
		if _, ok := h.profiles[p.ID]; ok || p.verify() != nil {
			return ErrHeld
		}
		h.profiles[p.ID] = p
	}
	h.observeStartup(ctx)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-done:
		}
	}()
	slots := make(chan struct{}, 16)
	for {
		c, e := listener.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				defer c.Close()
				stop := context.AfterFunc(ctx, func() { _ = c.Close() })
				defer stop()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				if peer(c) != nil {
					return
				}
				b, e := readRequest(c)
				var call Dispatch
				if e != nil || strict(b, &call) != nil {
					return
				}
				request, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if h.Dispatch(request, call) != nil {
					return
				}
				_, _ = io.WriteString(c, "accepted\n")
			}()
		default:
			_ = c.Close()
		}
	}
}

func DispatchLocal(ctx context.Context, socket string, call Dispatch) error {
	if !supported() {
		return ErrHeld
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, e := dialEndpoint(ctx, socket)
	if e != nil {
		return ErrHeld
	}
	defer c.Close()
	if peer(c) != nil {
		return ErrHeld
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	if len(call.Version)+len(call.ProfileID)+len(call.DescriptorRef)+len(call.DescriptorDigest) > maxFrame {
		return ErrHeld
	}
	payload, e := json.Marshal(call)
	if e != nil || len(payload)+1 > maxFrame {
		return ErrHeld
	}
	if writeRequest(c, append(payload, '\n')) != nil {
		return ErrHeld
	}
	b, e := io.ReadAll(io.LimitReader(c, 4097))
	if e != nil || string(b) != "accepted\n" {
		return ErrHeld
	}
	return nil
}

// DispatchDescriptor selects one owner profile by exact config path and bytes.
// Ambient client environment cannot select a daemon or a command.
func DispatchDescriptor(ctx context.Context, hostConfig, config, desc string) error {
	c, e := VerifyConfig(hostConfig)
	if e != nil {
		return e
	}
	for _, p := range c.Profiles {
		if p.ConfigRef == config {
			b, e := os.ReadFile(desc)
			var d descriptor
			if e != nil || strict(b, &d) != nil {
				return ErrHeld
			}
			call := Dispatch{DispatchVersion, p.ID, desc, d.DescriptorDigest}
			if validateDescriptor(p, call) != nil {
				return ErrHeld
			}
			return DispatchLocal(ctx, c.Socket, call)
		}
	}
	return ErrHeld
}
