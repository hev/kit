//go:build linux || darwin

package nativegate

import (
	"context"
	"encoding/json"
	"errors"
	n "github.com/hev/kit/pkg/nativejob"
	"os"
	"strings"
	"testing"
)

func TestOwnerProofStrictOriginalReadOnly(t *testing.T) {
	f := retainedSetup(t)
	j, err := f.store.Query(f.call.JobID)
	if err != nil {
		t.Fatal(err)
	}
	req := OwnerProofRequest{Version: n.OwnerProofVersion, JobID: j.ID, RequestDigest: n.RequestDigest(j.Request)}
	b, _ := json.Marshal(req)
	if _, err = DecodeOwnerProof(strings.NewReader(string(b))); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"version":"native-owner-proof/v1","job_id":null}`, strings.Replace(string(b), `"job_id"`, `"JOB_ID"`, 1), strings.Replace(string(b), `"job_id":`, `"job_id":"duplicate","job_id":`, 1), strings.Replace(string(b), `"version":`, `"argv":[],"version":`, 1), `{"version":"native-owner-proof/v1","job_id":"one"}`} {
		if _, err = DecodeOwnerProof(strings.NewReader(raw)); err == nil {
			t.Fatal("accepted malformed proof request", raw)
		}
	}
	out, err := InspectOwnerProof(context.Background(), f.cfg, req)
	if err != nil || out.JobID != j.ID || out.RequestDigest != req.RequestDigest || out.Enrollment != f.cfg.Enrollment || out.State != "Pending" || out.OwnerBirthStatus != "" || out.ChildBirthStatus != "" {
		t.Fatal(out, err)
	}
	metadata, err := InspectOwnerProof(context.Background(), f.cfg, OwnerProofRequest{Version: n.OwnerProofVersion})
	if err != nil || metadata.JobID != "" || metadata.Enrollment != f.cfg.Enrollment || metadata.Identity != nil || metadata.OwnerBirth != nil {
		t.Fatal(metadata, err)
	}
	req.RequestDigest = "sha256:" + strings.Repeat("0", 64)
	if _, err = InspectOwnerProof(context.Background(), f.cfg, req); err == nil {
		t.Fatal("accepted other original digest")
	}
	after, err := f.store.Query(j.ID)
	if err != nil || after.State != j.State || after.Owner != nil || after.StartReceipt != nil {
		t.Fatal("original changed", err)
	}
	if _, err = os.Stat(f.launches); !os.IsNotExist(err) {
		t.Fatal("proof launched worker")
	}
}

func TestOwnerProofBirthObservationIsConservative(t *testing.T) {
	b := n.Birth{BootID: "boot", PID: 42, Start: "123"}
	for _, tt := range []struct {
		want   string
		actual n.Birth
		err    error
	}{
		{"Alive", b, nil}, {"Reused", n.Birth{BootID: "other", PID: 42, Start: "123"}, nil},
		{"Gone", n.Birth{}, os.ErrNotExist}, {"Unknown", n.Birth{}, errors.New("denied or exited")},
	} {
		if got := observeBirth(b, func(int) (n.Birth, error) { return tt.actual, tt.err }); got != tt.want {
			t.Fatal(got, tt.want)
		}
	}
	if observeBirth(n.Birth{}, func(int) (n.Birth, error) { t.Fatal("read malformed birth"); return b, nil }) != "Unknown" {
		t.Fatal("malformed birth")
	}
}
