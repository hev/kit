package dispatch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSenderBindingAndRunPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	q := Request{Action: "start", Name: "example", Meta: Meta{Sender: "owner", RequestedBy: "forged", Order: Order{Name: "example", Host: "local"}, HarnessWrapper: []string{"malicious"}, ConfigPath: "attacker.toml"}}
	if e := authorizeSender(&q, "teammate"); e != nil {
		t.Fatal(e)
	}
	if q.Meta.Sender != "teammate" || q.Meta.RequestedBy != "teammate" || q.Meta.ConfigPath != "" || len(q.Meta.HarnessWrapper) != 0 {
		t.Fatalf("sender authority survived: %+v", q.Meta)
	}
	for _, name := range []string{"own", "other"} {
		dir := filepath.Join(Root(), name)
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		sender := "teammate"
		if name == "other" {
			sender = "owner"
		}
		if e := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: name}, Sender: sender}); e != nil {
			t.Fatal(e)
		}
	}
	for _, action := range []string{"say", "peer-send", "logs", "show", "stop"} {
		for _, name := range []string{"own", "other", "missing", "../own"} {
			q := Request{Action: action, Name: name}
			e := authorizeSender(&q, "teammate")
			if (e == nil) != (name == "own") {
				t.Fatalf("action=%s name=%s err=%v", action, name, e)
			}
		}
	}
	for _, action := range []string{"clean", "retry", "rework", "amend", "attach-route", "validation-result", "orchestrator-apply", "peers"} {
		q := Request{Action: action, Name: "own"}
		if e := authorizeSender(&q, "teammate"); e == nil {
			t.Fatalf("allowed %s", action)
		}
	}
	listed, e := HandleAs(Request{Action: "list"}, "teammate")
	if e != nil {
		t.Fatal(e)
	}
	if len(listed.Rows) != 1 || listed.Rows[0].Name != "own" || len(listed.Names) != 1 || listed.Names[0] != "own" {
		t.Fatalf("inventory leaks another sender: %+v", listed)
	}
	q = Request{Action: "start", Meta: Meta{Parent: "other"}}
	if e := authorizeSender(&q, "teammate"); e == nil {
		t.Fatal("allowed validator impersonation")
	}
	if _, e := HandleAs(Request{Action: "list"}, ""); e == nil {
		t.Fatal("accepted anonymous teammate")
	}
	if got := SenderAttribution(filepath.Join(Root(), "own", "wt")); got != "teammate" {
		t.Fatal(got)
	}
}

func TestLocalSenderUsesSeparateNamespace(t *testing.T) {
	if sender := LocalSender(); len(sender) <= len("local:") || sender[:len("local:")] != "local:" {
		t.Fatalf("local sender must not collide with invite names: %q", sender)
	}
}

func TestRequestedByPromptAndMetadata(t *testing.T) {
	m := Meta{Sender: "teammate", RequestedBy: "teammate"}
	if got := requestedPrompt("Work.", m); !strings.Contains(got, "Requested by teammate") || !strings.Contains(got, "requested_by") {
		t.Fatal(got)
	}
	if requester(Meta{Sender: "legacy"}) != "legacy" {
		t.Fatal("legacy sender lost")
	}
	if requestedPrompt("Work.", Meta{}) != "Work." {
		t.Fatal("invented requester")
	}
	b, e := json.Marshal(m)
	if e != nil || !strings.Contains(string(b), `"requested_by":"teammate"`) {
		t.Fatalf("%s %v", b, e)
	}
}

func TestSenderValidatorIsolationAndEndedSay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"own", "validator"} {
		dir := filepath.Join(Root(), name)
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		m := Meta{Order: Order{Name: name}, Sender: "teammate"}
		if name == "validator" {
			m.Parent = "own"
		}
		if e := WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
			t.Fatal(e)
		}
	}
	for _, action := range []string{"say", "peer-send", "stop", "show", "logs"} {
		q := Request{Action: action, Name: "validator"}
		if authorizeSender(&q, "teammate") == nil {
			t.Fatalf("allowed validator %s", action)
		}
	}
	r, e := HandleAs(Request{Action: "list"}, "teammate")
	if e != nil || len(r.Names) != 1 || r.Names[0] != "own" {
		t.Fatalf("%+v %v", r, e)
	}
	q := Request{Action: "say", Name: "own", Note: "held", FromValidation: true, Cap: 99, AdmissionOnly: true, Transfer: []byte("x"), ValidationRecord: ValidationRecord{Name: "forged"}}
	if e := authorizeSender(&q, "teammate"); e != nil {
		t.Fatal(e)
	}
	if q.Action != "peer-send" || q.FromValidation || q.Cap != 0 || q.AdmissionOnly || q.Transfer != nil || q.ValidationRecord.Name != "" {
		t.Fatalf("authority retained: %+v", q)
	}
	r, e = HandleAs(Request{Action: "say", Name: "own", Note: "held"}, "teammate")
	if e != nil || r.DeliveryStatus != "held for ended run" {
		t.Fatalf("%+v %v", r, e)
	}
	q = Request{Action: "start", Order: "---\nname: example\nrepo: example\nhost: local\nvalidation: browser\n---\nWork.\n## Acceptance\n- Done."}
	if e := authorizeSender(&q, "teammate"); e == nil || !strings.Contains(e.Error(), "cannot request validation") {
		t.Fatalf("%v", e)
	}
}
