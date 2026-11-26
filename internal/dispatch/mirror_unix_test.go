//go:build !windows

package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/pkg/kitapi"
)

func TestDispatchOrderMirrorLifecycle(t *testing.T) {
	h := newContextHarness(t)
	calls := make(chan kitapi.MirroredOrder, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var snapshot kitapi.MirroredOrder
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
			t.Error(err)
		}
		calls <- snapshot
		w.WriteHeader(204)
	}))
	defer server.Close()
	mirrorConfig(t, fmt.Sprintf("[kit]\napi=%q\nkey=%q\n[workspaces.client]\n", server.URL, mirrorTestKey))

	dir := filepath.Join(Root(), "mirrored-run")
	old := launchOrderMirror
	t.Cleanup(func() { launchOrderMirror = old })
	publishers := make(chan mirrorWatch, 16)
	launchOrderMirror = func(host Host, selected string, order *kitapi.MirroredOrder, watch bool, signIn mirrorSignIn) {
		publishers <- mirrorWatch{host, selected, *order, watch, signIn.fingerprint()}
	}
	runLauncher = func(dir, _ string) error {
		return writeProcessRecord(filepath.Join(dir, "supervisor.json"), os.Getpid())
	}
	call := func(q Request) Response {
		t.Helper()
		q.Name = "mirrored-run"
		res, err := RPC(context.Background(), Host{Local: true}, q)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	publisher := func(want string) mirrorWatch {
		t.Helper()
		select {
		case w := <-publishers:
			if w.Order.State != want {
				t.Fatalf("snapshot state %q, want %q", w.Order.State, want)
			}
			return w
		default:
			t.Fatalf("missing %s publisher", want)
		}
		return mirrorWatch{}
	}
	publish := func(w mirrorWatch, want ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		w.run(ctx, time.Millisecond, func(context.Context) (*kitapi.MirroredOrder, bool) {
			r, err := Handle(Request{Action: "mirror-snapshot", Name: w.Order.Name, WorkspaceName: w.Workspace})
			return r.MirroredOrder, err == nil
		})
		for _, state := range want {
			select {
			case got := <-calls:
				if got.State != state || got.Name != "mirrored-run" || got.Host != "here" ||
					got.Repo != "proj" || got.Base != "main" || got.Branch != "d/mirrored-run" ||
					got.Title != "Mirror lifecycle" || got.StartedAt.IsZero() {
					t.Errorf("wrong %s snapshot: %+v", state, got)
				}
			default:
				t.Errorf("missing %s update", state)
			}
		}
	}
	finish := func(outcome string, exit int) {
		t.Helper()
		if err := os.Remove(filepath.Join(dir, "supervisor.json")); err != nil {
			t.Fatal(err)
		}
		meta := runMeta(t, "mirrored-run")
		result := Result{Start: meta.Start, End: time.Now().UTC(), ExitCode: exit, Outcome: outcome,
			Branch: meta.Branch, Reason: "Waiting on operator"}
		if err := WriteJSON(filepath.Join(dir, "result.json"), result); err != nil {
			t.Fatal(err)
		}
	}
	order := strings.Replace(contextOrder("mirrored-run", "claude", ""), "repo: proj", "repo: proj\nhost: here", 1)
	order = strings.Replace(order, "Add the widget.", "# Mirror lifecycle", 1)
	o, err := Parse(order)
	if err != nil {
		t.Fatal(err)
	}
	call(Request{Action: "start", Order: order, Workspace: h.workspace, Meta: Meta{Order: o}})
	first := publisher("running")
	if r, err := Handle(Request{Action: "mirror-snapshot", Name: o.Name, WorkspaceName: "client"}); err == nil || r.MirroredOrder != nil {
		t.Fatal("client workspace accessed default workspace's snapshot")
	}
	finish("done", 0)
	publish(first, "running", "done")

	call(Request{Action: "rework", Note: "Keep going."})
	second := publisher("running")
	if !second.Order.StartedAt.After(first.Order.StartedAt) {
		t.Fatal("rework did not start a new attempt")
	}
	finish("blocked", 0)
	publish(second, "running", "blocked")
	call(Request{Action: "close", Note: "Finished."})
	publish(publisher("closed"), "closed")

	call(Request{Action: "rework", Note: "Try again."})
	_ = publisher("running")
	call(Request{Action: "stop"})
	stopping := publisher("halting")
	finish("", 143)
	publish(stopping, "halting", "failed")
}

func TestSSHDispatchMirrorDoesNotAwaitHostedService(t *testing.T) {
	hit := make(chan struct{}, 1)
	unblock := make(chan struct{})
	released := unblock
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		hit <- struct{}{}
		select {
		case <-released:
			w.WriteHeader(503)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer func() {
		if unblock != nil {
			close(unblock)
		}
	}()
	mirrorConfig(t, fmt.Sprintf("[kit]\napi=%q\nkey=%q\n", server.URL, mirrorTestKey))
	now := time.Now().UTC()
	snapshot := kitapi.MirroredOrder{Name: "ssh-order", Host: "oven", State: "done", StartedAt: now, EndedAt: now, UpdatedAt: now}
	response, err := json.Marshal(Response{Message: "started ssh-order", MirroredOrder: &snapshot})
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	spy := filepath.Join(bin, "request")
	t.Setenv("MIRROR_SSH_REQUEST", spy)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\ncat > \"$MIRROR_SSH_REQUEST\"\ncat <<'EOF_RESPONSE'\n" + string(response) + "\nEOF_RESPONSE\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	old := launchOrderMirror
	t.Cleanup(func() { launchOrderMirror = old })
	done := make(chan struct{})
	launchOrderMirror = func(host Host, selected string, order *kitapi.MirroredOrder, watch bool, signIn mirrorSignIn) {
		go func() {
			defer close(done)
			w := mirrorWatch{host, selected, *order, watch, signIn.fingerprint()}
			w.run(context.Background(), time.Second, nil)
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := RPC(ctx, Host{SSH: "fake-oven"}, Request{Action: "start", Name: "ssh-order"})
	if err != nil || res.Message != "started ssh-order" || ctx.Err() != nil {
		t.Fatalf("SSH dispatch did not finish independently: %q, %v", res.Message, err)
	}
	select {
	case <-hit:
	case <-ctx.Done():
		t.Fatal("mirror did not use the originating config")
	}
	raw, err := os.ReadFile(spy)
	if err != nil || strings.Contains(string(raw), mirrorTestKey) || !strings.Contains(string(raw), `"mirror_snapshot":true`) {
		t.Fatal("SSH RPC must request only metadata, without the hosted key")
	}
	close(unblock)
	unblock = nil
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("publisher did not return on 503")
	}
}
