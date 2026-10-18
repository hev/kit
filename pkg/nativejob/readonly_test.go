//go:build linux || darwin

package nativejob

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOnlyOriginalNeverEnrollsMissingOrDifferentJournal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	enrollment := Enrollment{HostID: "host", ExecutorID: "executor"}
	if _, e := ReadOnlyOriginal(context.Background(), root, enrollment, ""); e == nil {
		t.Fatal("missing journal accepted")
	}
	if _, e := os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("read created journal")
	}
	s, e := Open(root, enrollment)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e := ReadOnlyOriginal(context.Background(), root, enrollment, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := ReadOnlyOriginal(context.Background(), root, Enrollment{HostID: "foreign", ExecutorID: "executor"}, ""); e == nil {
		t.Fatal("different enrollment accepted")
	}
	var count int
	if e = s.db.QueryRow(`SELECT count(*) FROM jobs`).Scan(&count); e != nil || count != 0 {
		t.Fatal("read created job", count, e)
	}
}

func TestReadOnlyProofOmitsJobFactsForEnrollmentOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journal")
	enrollment := Enrollment{HostID: "host", ExecutorID: "executor"}
	s, e := Open(root, enrollment)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	proof, e := ReadOnlyProof(context.Background(), root, enrollment, "")
	if e != nil || proof.Version != OwnerProofVersion || proof.JobID != "" || proof.OwnerBirth != nil || proof.ChildBirth != nil || proof.Identity != nil || proof.Acknowledged != nil || proof.Completion != nil || proof.ObservedAt.IsZero() {
		t.Fatal("enrollment-only invented job facts", proof, e)
	}
	if _, e = ReadOnlyProof(context.Background(), root, enrollment, "missing-original"); e == nil {
		t.Fatal("missing job accepted")
	}
}

func TestReadOnlyProofOriginalCompletionBirthAndACKWithoutPrivatePayload(t *testing.T) {
	root := t.TempDir()
	s := testStore(t, filepath.Join(root, "journal"))
	testFence(t, s)
	r := testRequest(root)
	r.Argv = []string{"/bin/sh", "-c", ": 'PRIVATE_ARG_SENTINEL'; sleep 0.05; printf visible"}
	r.Environment = []string{"PRIVATE_ENV_SENTINEL=never-export"}
	r.PayloadDigest = RequestDigest(r)
	original := mustSubmit(t, s, r)
	pending, e := ReadOnlyProof(context.Background(), s.root, testEnrollment, original.ID)
	if e != nil || pending.State != "Pending" || pending.OwnerBirth != nil || pending.ChildBirth != nil {
		t.Fatal(pending, e)
	}
	completed, e := s.Execute(original.ID)
	if e != nil {
		t.Fatal(e)
	}
	proof, e := ReadOnlyProof(context.Background(), s.root, testEnrollment, original.ID)
	if e != nil || proof.Completion == nil || proof.OwnerBirth == nil || proof.ChildBirth == nil || *proof.OwnerBirth != *completed.Owner || *proof.ChildBirth != *completed.Birth || proof.RequestDigest != r.PayloadDigest || proof.Acknowledged == nil || *proof.Acknowledged {
		t.Fatal(proof, e)
	}
	raw, e := json.Marshal(proof)
	if e != nil {
		t.Fatal(e)
	}
	for _, forbidden := range []string{`"argv"`, `"environment"`, "PRIVATE_ARG_SENTINEL", "PRIVATE_ENV_SENTINEL", "never-export"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("owner proof leaked private payload")
		}
	}
	if e = s.Ack(original.ID, r.Identity, completed.EventID, completed.ResultDigest); e != nil {
		t.Fatal(e)
	}
	acked, e := ReadOnlyProof(context.Background(), s.root, testEnrollment, original.ID)
	if e != nil || acked.Acknowledged == nil || !*acked.Acknowledged || acked.Completion.EventID != proof.Completion.EventID {
		t.Fatal(acked, e)
	}
	// Persisted births remain historical after exit; no live/adopted claim added.
	if _, e = ProcessBirth(completed.Birth.PID); e == nil {
		t.Fatal("owned completed process still live")
	}
	completed.ResultDigest = "corrupt"
	if _, e = s.db.Exec(`UPDATE jobs SET body=? WHERE id=?`, encode(completed), completed.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = ReadOnlyProof(context.Background(), s.root, testEnrollment, completed.ID); e == nil {
		t.Fatal("corrupt terminal digest accepted")
	}
}
