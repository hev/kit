package nativegate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	n "github.com/hev/kit/pkg/nativejob"
	"io"
	"os"
	"path/filepath"
	"time"
)

// OwnerProofRequest selects only an authenticated original journal record.
// Empty job/digest authenticate enrollment alone.
type OwnerProofRequest struct {
	Version       string `json:"version"`
	JobID         string `json:"job_id,omitempty"`
	RequestDigest string `json:"request_digest,omitempty"`
}

type OwnerProof struct {
	n.OwnerProof
	OwnerBirthStatus string `json:"owner_birth_status,omitempty"`
	ChildBirthStatus string `json:"child_birth_status,omitempty"`
}

func DecodeOwnerProof(r io.Reader) (OwnerProofRequest, error) {
	var request OwnerProofRequest
	raw, err := decodeExact(r, &request)
	if err != nil {
		return request, ErrReceiverCall
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return request, ErrReceiverCall
	}
	for _, key := range []string{"job_id", "request_digest"} {
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return request, ErrReceiverCall
		}
	}
	if request.Version != n.OwnerProofVersion || (request.JobID == "") != (request.RequestDigest == "") || (request.RequestDigest != "" && !validFileDigest(request.RequestDigest)) {
		return request, ErrReceiverCall
	}
	return request, nil
}

// InspectOwnerProof never initializes, migrates, recovers, acknowledges, adopts
// or kills a process. Individual process observations do not prove tree absence.
func InspectOwnerProof(ctx context.Context, cfg ReceiverConfig, request OwnerProofRequest) (OwnerProof, error) {
	var out OwnerProof
	if request.Version != n.OwnerProofVersion || (request.JobID == "") != (request.RequestDigest == "") || cfg.Version != ReceiverVersion || privateDirectory(cfg.JournalRoot) != nil || privateFile(filepath.Join(cfg.JournalRoot, "journal.sqlite")) != nil {
		return out, ErrReceiverCall
	}
	proof, err := n.ReadOnlyProof(ctx, cfg.JournalRoot, cfg.Enrollment, request.JobID)
	if err != nil {
		return out, err
	}
	if request.JobID != "" && proof.RequestDigest != request.RequestDigest {
		return out, ErrReceiverCall
	}
	out.OwnerProof = proof
	if proof.OwnerBirth != nil {
		out.OwnerBirthStatus = observeBirth(*proof.OwnerBirth, n.ProcessBirth)
	}
	if proof.ChildBirth != nil {
		out.ChildBirthStatus = observeBirth(*proof.ChildBirth, n.ProcessBirth)
	}
	if err = ctx.Err(); err != nil {
		return OwnerProof{}, err
	}
	out.ObservedAt = time.Now().UTC()
	return out, nil
}

func observeBirth(expected n.Birth, read func(int) (n.Birth, error)) string {
	if expected.PID <= 0 || expected.BootID == "" || expected.Start == "" {
		return "Unknown"
	}
	current, err := read(expected.PID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "Gone"
		}
		return "Unknown"
	}
	if current != expected {
		return "Reused"
	}
	return "Alive"
}
