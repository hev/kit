package nativegate

import (
	"context"
	"io"
	"os"
	"path/filepath"

	a "github.com/hev/factory/pkg/nativeadmission"
	h "github.com/hev/kit/pkg/nativehost"
	n "github.com/hev/kit/pkg/nativejob"
)

func DecodeHostRegistration(r io.Reader) (h.RegistrationRequest, error) {
	var request h.RegistrationRequest
	if _, e := decodeExact(r, &request); e != nil {
		return request, ErrReceiverCall
	}
	if request.Version != h.RegistrationVersion || request.Enrollment.HostID == "" || request.Enrollment.ExecutorID == "" || request.NodeUID == "" || request.Incarnation == "" || request.AdmissionGeneration == 0 || (request.DescriptorRef == "") != (request.DescriptorDigest == "") {
		return request, ErrReceiverCall
	}
	return request, nil
}

// InspectHostRegistration observes existing journal enrollment and maintenance
// authority. It never enrolls, issues a fence, claims, spawns or connects to PG.
func InspectHostRegistration(ctx context.Context, cfg ReceiverConfig, request h.RegistrationRequest) (h.Registration, error) {
	return inspectHostRegistration(ctx, cfg, request, a.InspectNodeRegistration)
}
func inspectHostRegistration(ctx context.Context, cfg ReceiverConfig, request h.RegistrationRequest, inspect func(context.Context, a.NodeRegistrationConfig) (a.NodeRegistration, error)) (h.Registration, error) {
	var out h.Registration
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if (request.DescriptorRef == "") != (request.DescriptorDigest == "") {
		return out, ErrReceiverCall
	}
	if cfg.Version != ReceiverVersion || request.Version != h.RegistrationVersion || request.Enrollment != cfg.Enrollment || request.NodeUID == "" || request.Incarnation == "" || request.AdmissionGeneration == 0 || privateDirectory(cfg.JournalRoot) != nil {
		return out, ErrReceiverCall
	}
	// Owner-private files are checked before read-only SQLite access.
	if privateFile(filepath.Join(cfg.JournalRoot, "journal.sqlite")) != nil {
		return out, ErrReceiverCall
	}
	id := ""
	var descriptor NativeSupervisorDescriptor
	if request.DescriptorRef != "" || request.DescriptorDigest != "" {
		if cfg.Retained == nil || privateJSON(request.DescriptorRef, &descriptor) != nil || descriptor.DescriptorDigest != request.DescriptorDigest || DescriptorDigest(descriptor) != request.DescriptorDigest || canonicalStart(descriptor.StartCall) != nil || request.DescriptorRef != descriptorPath(cfg, descriptor.StartCall.JobID) {
			return out, ErrReceiverCall
		}
		if verifyRetainedSelection(descriptor.ConfigRef, cfg) != nil {
			return out, ErrReceiverCall
		}
		configHash, err := fileDigest(descriptor.ConfigRef)
		if err != nil || configHash != descriptor.ConfigDigest || descriptor.SupervisorRef != cfg.Retained.Supervisor || descriptor.SupervisorDigest != cfg.Retained.SupervisorDigest || descriptor.WorkloadTimeoutSeconds != cfg.Retained.WorkloadTimeoutSeconds {
			return out, ErrReceiverCall
		}
		id = descriptor.StartCall.JobID
	}
	original, e := n.ReadOnlyOriginal(ctx, cfg.JournalRoot, cfg.Enrollment, id)
	if e != nil {
		return out, e
	}
	if id != "" {
		c := descriptor.StartCall
		b := c.Binding
		identity := original.Request.Identity
		authority := original.Request.StartAuthority
		if original.State != "Pending" || authority == nil || authority.NodeUID != request.NodeUID || authority.Incarnation != request.Incarnation || authority.AdmissionGeneration != request.AdmissionGeneration || identity.HostID != request.Enrollment.HostID || identity.ExecutorID != request.Enrollment.ExecutorID || b == nil || b.JobID != original.ID || b.RequestDigest != n.RequestDigest(original.Request) || b.RequestDigest != original.Request.PayloadDigest || b.OrderUID != identity.OrderUID || b.ScopeDigest != identity.ScopeDigest {
			return out, ErrReceiverCall
		}
	}
	if inspect == nil {
		return out, ErrReceiverCall
	}
	binding := a.NativeNodeBinding{HostID: request.Enrollment.HostID, ExecutorID: request.Enrollment.ExecutorID, NodeUID: request.NodeUID, Incarnation: request.Incarnation, AdmissionGeneration: request.AdmissionGeneration}
	observed, e := inspect(ctx, a.NodeRegistrationConfig{MaintenanceDir: cfg.MaintenanceDir, MaintenanceOwner: cfg.MaintenanceOwner, Workspace: cfg.Workspace, Parent: cfg.Parent, Binding: binding})
	if e != nil {
		return out, e
	}
	if observed.Binding != binding || observed.ObservedAt.IsZero() {
		return out, ErrReceiverCall
	}
	return h.Registration{Version: h.RegistrationVersion, Enrollment: cfg.Enrollment, NodeUID: binding.NodeUID, Incarnation: binding.Incarnation, AdmissionGeneration: binding.AdmissionGeneration, ObservedAt: observed.ObservedAt}, nil
}

func privateFile(path string) error {
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() || !ownedByCurrentUser(s) {
		return ErrReceiverCall
	}
	return nil
}
