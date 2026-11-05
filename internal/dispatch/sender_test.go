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
	for _, action := range []string{"clean", "retry", "rework", "amend", "attach-route", "validation-result", "orchestrator-apply"} {
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
