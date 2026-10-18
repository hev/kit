package nativegate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	n "github.com/hev/kit/pkg/nativejob"
)

var ErrRetainedUnknown = errors.New("retained native outcome unknown; inspect original journal; do not replay")

// RunReceiverFromConfig selects retention only from the authenticated private
// configuration. The normal inspection/ACK wire remains unchanged.
func RunReceiverFromConfig(ctx context.Context, path string, cfg ReceiverConfig, call ReceiverCall) (Inspection, error) {
	if cfg.Retained != nil && call.Operation == "start" {
		return startRetained(ctx, path, cfg, call)
	}
	return RunReceiver(ctx, cfg, call)
}
func descriptorPath(cfg ReceiverConfig, id string) string {
	return filepath.Join(cfg.Retained.StateRoot, strings.TrimPrefix(n.Digest(id), "sha256:"), "descriptor.json")
}
func verifyRetainedSelection(configPath string, cfg ReceiverConfig) error {
	if cfg.Retained == nil || !cfg.Retained.valid() {
		return ErrReceiverCall
	}
	actual, err := LoadReceiverConfig(configPath)
	if err != nil || n.Digest(actual) != n.Digest(cfg) {
		return ErrReceiverCall
	}
	for _, p := range []string{cfg.Retained.StateRoot, cfg.JournalRoot} {
		if err = privateDirectory(p); err != nil {
			return err
		}
	}
	if err = pinnedExecutable(cfg.Retained.Launcher, cfg.Retained.LauncherDigest); err != nil {
		return err
	}
	return pinnedExecutable(cfg.Retained.Supervisor, cfg.Retained.SupervisorDigest)
}
func openOriginal(cfg ReceiverConfig) (*n.Store, error) {
	if err := privateDirectory(cfg.JournalRoot); err != nil {
		return nil, err
	}
	st, err := os.Lstat(filepath.Join(cfg.JournalRoot, "journal.sqlite"))
	if err != nil || !st.Mode().IsRegular() || !ownedByCurrentUser(st) {
		return nil, ErrReceiverCall
	}
	return n.Open(cfg.JournalRoot, cfg.Enrollment)
}
func boundInspection(i Inspection, c ReceiverCall) bool {
	return c.Binding != nil && i.JobID == c.JobID && i.JobID == c.Binding.JobID && i.Identity.OrderUID == c.Binding.OrderUID && i.Identity.ScopeDigest == c.Binding.ScopeDigest && i.RequestDigest == c.Binding.RequestDigest && i.PayloadDigest == i.RequestDigest && i.Authority != nil
}

func startRetained(ctx context.Context, configPath string, cfg ReceiverConfig, call ReceiverCall) (Inspection, error) {
	var empty Inspection
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := canonicalStart(call); err != nil {
		return empty, err
	}
	if err := verifyRetainedSelection(configPath, cfg); err != nil {
		return empty, err
	}
	store, err := openOriginal(cfg)
	if err != nil {
		return empty, err
	}
	defer store.Close()
	original, err := Inspect(ctx, store, call.JobID)
	if err != nil || !boundInspection(original, call) {
		return empty, ErrRetainedUnknown
	}
	digest, err := fileDigest(configPath)
	if err != nil {
		return empty, err
	}
	descriptor := NativeSupervisorDescriptor{Version: SupervisorVersion, ConfigRef: configPath, ConfigDigest: digest, SupervisorRef: cfg.Retained.Supervisor, SupervisorDigest: cfg.Retained.SupervisorDigest, StartCall: call, WorkloadTimeoutSeconds: cfg.Retained.WorkloadTimeoutSeconds}
	descriptor.DescriptorDigest = DescriptorDigest(descriptor)
	path := descriptorPath(cfg, call.JobID)
	dir := filepath.Dir(path)
	created := false
	if err = os.Mkdir(dir, 0700); err == nil {
		if err = syncDirectory(cfg.Retained.StateRoot); err != nil {
			return empty, err
		}
	} else if !os.IsExist(err) {
		return empty, err
	}
	if err = privateDirectory(dir); err != nil {
		return empty, err
	}
	// Only the creator may launch. Existing partial/uncertain descriptors are
	// retained, never truncated, scanned or retried as executable work.
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		if original.State != "Pending" {
			return empty, ErrRetainedUnknown
		}
		id := original.Identity
		if _, err = store.IssuedFence(id.OrderUID, id.ChefID, id.Fence); err != nil {
			return empty, err
		}
		err = exclusiveJSON(path, descriptor)
		if err == nil {
			created = true
		} else if !os.IsExist(err) {
			return empty, ErrRetainedUnknown
		}
	} else if err != nil {
		return empty, ErrRetainedUnknown
	}
	var retained NativeSupervisorDescriptor
	if err = privateJSON(path, &retained); err != nil || retained.DescriptorDigest != descriptor.DescriptorDigest || DescriptorDigest(retained) != descriptor.DescriptorDigest {
		return empty, ErrRetainedUnknown
	}
	if created {
		cmd := exec.CommandContext(ctx, cfg.Retained.Launcher, "--config", configPath, "--descriptor", path)
		var out launcherOutput
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
		cmd.WaitDelay = 2 * time.Second
		if err = cmd.Run(); err != nil || strings.TrimSpace(out.String()) != "accepted" {
			return empty, ErrRetainedUnknown
		}
	}
	return waitReadiness(ctx, store, path, descriptor)
}

type launcherOutput struct{ bytes.Buffer }

func (b *launcherOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, ErrRetainedUnknown
	}
	return b.Buffer.Write(p)
}

func waitReadiness(ctx context.Context, store *n.Store, path string, d NativeSupervisorDescriptor) (Inspection, error) {
	readyPath := filepath.Join(filepath.Dir(path), "ready.json")
	for {
		if err := ctx.Err(); err != nil {
			return Inspection{}, ErrRetainedUnknown
		}
		j, err := store.Recover(d.StartCall.JobID)
		if err != nil {
			return Inspection{}, ErrRetainedUnknown
		}
		i, err := Inspect(ctx, store, d.StartCall.JobID)
		if err != nil || !boundInspection(i, d.StartCall) {
			return Inspection{}, ErrRetainedUnknown
		}
		if i.State == "Completed" && i.Completion != nil {
			return i, nil
		}
		var ready NativeSupervisorReady
		if _, err = os.Lstat(readyPath); err == nil {
			if err = privateJSON(readyPath, &ready); err != nil || ready.Version != SupervisorReadyVersion || ready.DescriptorDigest != d.DescriptorDigest {
				return Inspection{}, ErrRetainedUnknown
			}
			if err = syncDirectory(filepath.Dir(readyPath)); err != nil {
				return Inspection{}, ErrRetainedUnknown
			}
			o := ready.Observation
			if o.JobID != j.ID || o.PayloadDigest != j.Request.PayloadDigest || j.Owner == nil || o.Owner != *j.Owner || (o.Child == nil) != (j.Birth == nil) || (o.Child != nil && *o.Child != *j.Birth) || o.StartReceipt == nil || j.StartReceipt == nil || n.Digest(o.StartReceipt) != n.Digest(j.StartReceipt) {
				return Inspection{}, ErrRetainedUnknown
			}
			if i.State == "Running" && o.Child != nil && liveBirth(o.Owner) && liveBirth(*o.Child) {
				return i, nil
			}
		} else if !os.IsNotExist(err) {
			return Inspection{}, ErrRetainedUnknown
		}
		if i.State == "Uncertain" && (j.Owner == nil || !liveBirth(*j.Owner)) {
			return Inspection{}, ErrRetainedUnknown
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Inspection{}, ErrRetainedUnknown
		case <-timer.C:
		}
	}
}
func liveBirth(b n.Birth) bool { got, err := n.ProcessBirth(b.PID); return err == nil && got == b }

func loadSupervisorDescriptor(configPath string, cfg ReceiverConfig, path string) (NativeSupervisorDescriptor, error) {
	var d NativeSupervisorDescriptor
	if err := verifyRetainedSelection(configPath, cfg); err != nil {
		return d, err
	}
	if !canonicalPath(path) || privateDirectory(filepath.Dir(path)) != nil {
		return d, ErrReceiverCall
	}
	if err := privateJSON(path, &d); err != nil {
		return d, err
	}
	digest, err := fileDigest(configPath)
	if err != nil || d.Version != SupervisorVersion || d.ConfigRef != configPath || d.ConfigDigest != digest || d.SupervisorRef != cfg.Retained.Supervisor || d.SupervisorDigest != cfg.Retained.SupervisorDigest || d.WorkloadTimeoutSeconds != cfg.Retained.WorkloadTimeoutSeconds || d.DescriptorDigest != DescriptorDigest(d) || path != descriptorPath(cfg, d.StartCall.JobID) || canonicalStart(d.StartCall) != nil {
		return d, ErrReceiverCall
	}
	executable, err := os.Executable()
	if err != nil || filepath.Clean(executable) != d.SupervisorRef {
		return d, ErrReceiverCall
	}
	return d, nil
}

// Supervise is called only by the fixed retained executable entrypoint. It does
// not inherit the start client's lifetime and never replays a consumed descriptor.
// The installed launcher is responsible for separate process/cgroup ownership.
func Supervise(ctx context.Context, configPath string, cfg ReceiverConfig, path string) (Inspection, error) {
	return supervise(ctx, configPath, cfg, path, func(ctx context.Context, cfg ReceiverConfig, c ReceiverCall, observer func(n.StartedObservation)) (Inspection, error) {
		return runReceiver(ctx, cfg, c, true, observer)
	})
}
func supervise(ctx context.Context, configPath string, cfg ReceiverConfig, path string, run func(context.Context, ReceiverConfig, ReceiverCall, func(n.StartedObservation)) (Inspection, error)) (Inspection, error) {
	d, err := loadSupervisorDescriptor(configPath, cfg, path)
	if err != nil {
		return Inspection{}, err
	}
	owner, err := n.ProcessBirth(os.Getpid())
	if err != nil {
		return Inspection{}, err
	}
	claim := struct {
		Version          string  `json:"version"`
		DescriptorDigest string  `json:"descriptor_digest"`
		Owner            n.Birth `json:"owner"`
	}{SupervisorVersion, d.DescriptorDigest, owner}
	if err = exclusiveJSON(filepath.Join(filepath.Dir(path), "supervisor.json"), claim); err != nil {
		return Inspection{}, ErrRetainedUnknown
	}
	// Publication is best effort. Failure must not interrupt the original Wait or
	// completion. A missing readiness reply leaves the caller inspect-only.
	return run(ctx, cfg, d.StartCall, func(observation n.StartedObservation) {
		if observation.Owner != owner {
			return
		}
		ready := NativeSupervisorReady{Version: SupervisorReadyVersion, DescriptorDigest: d.DescriptorDigest, Observation: observation}
		// Notification storage cannot hold up the original process Wait. A fast
		// completion may exit without a ready file; its durable result is enough.
		go func() { _ = exclusiveJSON(filepath.Join(filepath.Dir(path), "ready.json"), ready) }()
	})
}
