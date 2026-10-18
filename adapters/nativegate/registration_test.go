//go:build linux || darwin

package nativegate

import (
	"context"
	"encoding/json"
	a "github.com/hev/factory/pkg/nativeadmission"
	h "github.com/hev/kit/pkg/nativehost"
	n "github.com/hev/kit/pkg/nativejob"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegistrationAuthenticatesOriginalBeforeNeutralProjection(t *testing.T) {
	f := retainedSetup(t)
	j, e := f.store.Query(f.call.JobID)
	if e != nil {
		t.Fatal(e)
	}
	authority := j.Request.StartAuthority
	if authority == nil {
		t.Fatal("fixture lacks nativeauthority")
	}
	configHash, _ := fileDigest(f.config)
	d := NativeSupervisorDescriptor{Version: SupervisorVersion, ConfigRef: f.config, ConfigDigest: configHash, SupervisorRef: f.cfg.Retained.Supervisor, SupervisorDigest: f.cfg.Retained.SupervisorDigest, StartCall: f.call, WorkloadTimeoutSeconds: 10}
	d.DescriptorDigest = DescriptorDigest(d)
	path := descriptorPath(f.cfg, f.call.JobID)
	if e = os.Mkdir(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e = exclusiveJSON(path, d); e != nil {
		t.Fatal(e)
	}
	request := h.RegistrationRequest{Version: h.RegistrationVersion, Enrollment: f.cfg.Enrollment, NodeUID: authority.NodeUID, Incarnation: authority.Incarnation, AdmissionGeneration: authority.AdmissionGeneration, DescriptorRef: path, DescriptorDigest: d.DescriptorDigest}
	calls := 0
	inspect := func(ctx context.Context, c a.NodeRegistrationConfig) (a.NodeRegistration, error) {
		calls++
		return a.NodeRegistration{Binding: c.Binding, ObservedAt: time.Now().UTC()}, nil
	}
	got, e := inspectHostRegistration(context.Background(), f.cfg, request, inspect)
	if e != nil || got.NodeUID != authority.NodeUID || calls != 1 {
		t.Fatal(got, e, calls)
	}
	for _, change := range []func(*h.RegistrationRequest){func(r *h.RegistrationRequest) { r.NodeUID = "workflow-node" }, func(r *h.RegistrationRequest) { r.Incarnation = "stale" }, func(r *h.RegistrationRequest) { r.AdmissionGeneration++ }, func(r *h.RegistrationRequest) { r.Enrollment.HostID = "foreign" }, func(r *h.RegistrationRequest) { r.DescriptorDigest = "sha256:" + strings.Repeat("0", 64) }, func(r *h.RegistrationRequest) { r.DescriptorRef = "" }} {
		bad := request
		change(&bad)
		if _, e = inspectHostRegistration(context.Background(), f.cfg, bad, inspect); e == nil {
			t.Fatal("forged mapping accepted")
		}
		if calls != 1 {
			t.Fatal("projection used before original validation")
		}
	}
	original, e := f.store.Query(f.call.JobID)
	if e != nil || original.State != "Pending" || original.StartReceipt != nil || original.Owner != nil {
		t.Fatal("registration changed original", original.State, e)
	}
	events, e := f.store.Outbox()
	if e != nil || len(events) != 0 {
		t.Fatal("registration produced result", e)
	}
	if _, e = os.Stat(f.launches); !os.IsNotExist(e) {
		t.Fatal("registration launched")
	}
}
func TestRegistrationStrictRequest(t *testing.T) {
	r := h.RegistrationRequest{Version: h.RegistrationVersion, Enrollment: n.Enrollment{HostID: "host", ExecutorID: "executor"}, NodeUID: "node", Incarnation: "inc", AdmissionGeneration: 2}
	b, _ := json.Marshal(r)
	if _, e := DecodeHostRegistration(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{strings.Replace(string(b), `"node_uid"`, `"NODE_UID"`, 1), strings.Replace(string(b), `"node_uid":"node"`, `"node_uid":"node","node_uid":"other"`, 1), strings.Replace(string(b), `"version"`, `"argv":[],"version"`, 1)} {
		if _, e := DecodeHostRegistration(strings.NewReader(raw)); e == nil {
			t.Fatal("ambiguous request accepted")
		}
	}
}
