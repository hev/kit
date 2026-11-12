package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hev/kit/internal/update"
)

type updateCall struct {
	q  update.Request
	by string
}

func stubUpdateHost(t *testing.T) *[]updateCall {
	t.Helper()
	var calls []updateCall
	saved := UpdateHost
	UpdateHost = func(_ context.Context, q update.Request, by string) (update.Report, error) {
		calls = append(calls, updateCall{q, by})
		return update.Report{Before: "0.7.0-edge.aaaaaaa", After: "0.7.0-edge.bbbbbbb", Changed: true, Message: "updated"}, nil
	}
	t.Cleanup(func() { UpdateHost = saved })
	return &calls
}

// An invited member's key reaches the host through HandleGrant; updating
// is an owner action, so it is refused before anything runs.
func TestUpdateRequestFromMemberIsRefused(t *testing.T) {
	calls := stubUpdateHost(t)
	for _, allow := range [][]string{nil, {RepoCapability}} {
		_, err := HandleGrant(Request{Action: "update", Channel: update.ChannelEdge}, "member:teammate", allow)
		if err == nil || !strings.Contains(err.Error(), "owner action") {
			t.Fatalf("member update: err = %v", err)
		}
	}
	if _, err := HandleAs(Request{Action: "update"}, "member:teammate"); err == nil {
		t.Fatal("HandleAs accepted update")
	}
	if len(*calls) != 0 {
		t.Fatalf("the host ran an update for a member: %+v", *calls)
	}
}

// The owner's request reaches the updater with a channel, a version and the
// check flag, nothing else, whatever else the JSON carries.
func TestUpdateRequestCarriesOnlyChannelAndVersion(t *testing.T) {
	calls := stubUpdateHost(t)
	raw := `{"action":"update","channel":"edge","to":"v0.7.0-edge.bbbbbbb","check":false,
		"url":"https://attacker.example/hev","transfer":"aGV2","order":"binary: /tmp/evil\n",
		"note":"sha256=0000","files":{"hev":"evil"},"meta":{"sender":"local:owner","name":"x"},
		"digest":"00","binary":"/tmp/evil","source":"https://attacker.example"}`
	var q Request
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatal(err)
	}
	r, err := Handle(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %+v", *calls)
	}
	got := (*calls)[0]
	want := update.Request{Channel: "edge", To: "v0.7.0-edge.bbbbbbb", SaveChannel: "edge"}
	if got.q != want || got.by != "dispatch:local:owner" {
		t.Fatalf("updater got %+v by %q, want %+v", got.q, got.by, want)
	}
	if r.Update == nil || r.Update.After != "0.7.0-edge.bbbbbbb" {
		t.Fatalf("reply = %+v", r)
	}
	// A version that is not one (a URL, a path) or an unknown channel never
	// reaches the updater.
	for _, bad := range []Request{
		{Action: "update", To: "https://attacker.example/hev"},
		{Action: "update", To: "../../bin/sh"},
		{Action: "update", Channel: "https://attacker.example"},
	} {
		if _, err := Handle(bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if len(*calls) != 1 {
		t.Fatal("a refused request reached the updater")
	}
}

// A host on a release from before this request answers it as a run with no
// name; the sender says what that means and what to do.
func TestUpdateErrorFromOlderHostIsClear(t *testing.T) {
	err := UpdateError("host-a", Response{Version: "0.6.2", Error: "invalid name"}, errors.New("invalid name"))
	if !strings.Contains(err.Error(), "host-a runs hev 0.6.2, which cannot update itself on request") || !strings.Contains(err.Error(), "by hand once") {
		t.Fatal(err)
	}
	other := errors.New("digest mismatch")
	if UpdateError("host-a", Response{Update: &update.Report{}}, other) != other {
		t.Fatal("a real failure was rewritten")
	}
}

// Hosts update at different moments: an older sender ignores the reply's
// channel, and an older host's reply decodes with none.
func TestUpdateFieldsAreCompatibleOneVersionBack(t *testing.T) {
	b, _ := json.Marshal(Response{Cap: 2, Channel: update.ChannelEdge, Update: &update.Report{After: "0.7.0"}})
	var old struct {
		Cap int `json:"cap"`
	}
	if err := json.Unmarshal(b, &old); err != nil || old.Cap != 2 {
		t.Fatal(err)
	}
	var fromOld Response
	if err := json.Unmarshal([]byte(`{"cap":2,"version":"0.6.2","rows":[]}`), &fromOld); err != nil || fromOld.Channel != "" || fromOld.Update != nil {
		t.Fatal(err)
	}
	// An older host decodes the new request fields away and answers the
	// action as it always did (see TestUpdateErrorFromOlderHostIsClear).
	var oldReq struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	b, _ = json.Marshal(Request{Action: "update", Channel: "edge", To: "0.7.0"})
	if err := json.Unmarshal(b, &oldReq); err != nil || oldReq.Action != "update" || oldReq.Name != "" {
		t.Fatal(err)
	}
}
