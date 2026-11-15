package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	boardstore "github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/hev/kit/pkg/kitapi/kitapitest"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAPIKeyFreshHomeCaptureSearchBoardAndDaemon(t *testing.T) {
	home := kitE2EHome(t)
	api := kitapitest.New(t, fakeKitKey, e2eIdentity)
	ctx := context.Background()

	var out bytes.Buffer
	if err := os.Remove(daemon.DefaultConfigPath()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_API_KEY", fakeKitKey)
	t.Setenv(kitServiceEnv, api.URL)

	k := kitSignedIn()
	if k == nil || k.API != api.URL {
		t.Fatalf("not signed in to the fake API: %+v", k)
	}

	// The CLI adopted the env key; a service manager starts without that env.
	info, err := os.Stat(daemon.DefaultConfigPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private config missing")
	}
	t.Setenv("HEV_API_KEY", "")
	t.Setenv(kitServiceEnv, "")
	// Capture.
	const session = "e2e00000-0000-4000-8000-000000000001"
	writeClaudeTranscript(t, home, session, "zebra mango handshake")
	runCaptureCycle(t)
	rows, err := api.Traces.ListSessionRows(-1, nil)
	if err != nil || len(rows) != 1 || rows[0].SessionID != session {
		t.Fatalf("API sessions = %+v, %v", rows, err)
	}
	if rows[0].AuthorID != "user_9" || rows[0].Host != "box" {
		t.Fatalf("session not stamped by the key: author=%q host=%q", rows[0].AuthorID, rows[0].Host)
	}
	if blocks, _ := api.Traces.ListBlockRows(session); len(blocks) == 0 {
		t.Fatal("no blocks reached the API")
	}

	// Search through the API, the way `hev traces search --json` does.
	out.Reset()
	queryJSON = true
	t.Cleanup(func() { queryJSON = false })
	queryCmd.SetContext(ctx)
	queryCmd.SetOut(&out)
	t.Cleanup(func() { queryCmd.SetOut(nil) })
	if err := queryCmd.RunE(queryCmd, []string{"zebra", "mango", "handshake"}); err != nil {
		t.Fatal(err)
	}
	var hits []kitapi.SearchHit
	if err := json.Unmarshal(out.Bytes(), &hits); err != nil || len(hits) != 1 || hits[0].ID != session || !strings.Contains(hits[0].Snippet, "zebra mango handshake") {
		t.Fatalf("search = %s (%v)", out.String(), err)
	}
	if api.Requests("GET /v1/traces/search") == 0 {
		t.Fatal("search did not go through the kit API")
	}

	// The local dashboard reads through the API.
	dash := httptest.NewServer(kitDashboard(kitClient(k)))
	defer dash.Close()
	resp, err := http.Get(dash.URL + "/api/session/" + session)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("dashboard session: %v %v", resp, err)
	}
	resp.Body.Close()

	// Board post round-trips through the API.
	stdout := captureStdout(t, func() {
		if err := runBoard([]string{"post", "--json", "--board", "floor", "--title", "kit e2e", "-m", "posted through the kit API"}); err != nil {
			t.Fatal(err)
		}
	})
	var th kitapi.Thread
	if err := json.Unmarshal([]byte(stdout), &th); err != nil || th.ID == 0 || th.Author != "Ada" {
		t.Fatalf("post = %s (%v)", stdout, err)
	}
	stdout = captureStdout(t, func() {
		if err := runBoard([]string{"read", "--json", fmt.Sprint(th.ID)}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "posted through the kit API") {
		t.Fatalf("read = %s", stdout)
	}
	if got, _ := api.Board.Threads(ctx, boardstore.ListOpts{}); len(got) != 1 {
		t.Fatalf("API board threads = %d", len(got))
	}

	// hev open asks the API for a one-time link.
	old := inviteOpenURL
	t.Cleanup(func() { inviteOpenURL = old })
	var opened string
	inviteOpenURL = func(_ io.Writer, u string) error { opened = u; return nil }
	if err := runOpen(ctx, &out); err != nil || !strings.HasPrefix(opened, api.URL+"/s/") {
		t.Fatalf("open = %q, %v", opened, err)
	}

}

func TestAPIKeyHelperVerifyStoreRefuseLogoutAndNoLeak(t *testing.T) {
	kitE2EHome(t)
	api := kitapitest.New(t, fakeKitKey, e2eIdentity)
	var out bytes.Buffer
	if err := runKeyLogin(context.Background(), &out, api.URL, fakeKitKey); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(daemon.DefaultConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(daemon.DefaultConfigPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("not 0600")
	}
	err = runKeyLogin(context.Background(), &out, api.URL, "refused-fixture")
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatal("refused key accepted")
	}
	after, _ := os.ReadFile(daemon.DefaultConfigPath())
	if !bytes.Equal(before, after) {
		t.Fatal("refused key changed config")
	}
	configShowCmd.SetOut(&out)
	defer configShowCmd.SetOut(nil)
	if err := configShowCmd.RunE(configShowCmd, nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEV_API_KEY", fakeKitKey)
	if err := runKeyLogout(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "HEV_API_KEY still applies") {
		t.Fatal("no env warning")
	}
	raw, _ := os.ReadFile(daemon.DefaultConfigPath())
	if bytes.Contains(raw, []byte(fakeKitKey)) {
		t.Fatal("stored key remains")
	}
	if strings.Contains(out.String(), fakeKitKey) || strings.Contains(out.String(), "refused-fixture") {
		t.Fatal("output contains credential")
	}
	t.Setenv("HEV_API_KEY", "")
	err = runKeyLogin(context.Background(), &out, api.URL, "")
	if err == nil || !strings.Contains(err.Error(), daemon.HostedSetup) {
		t.Fatal("no setup message")
	}
}

func TestAPIKey401KeepsOutboxAndStoredKey(t *testing.T) {
	kitE2EHome(t)
	api := kitapitest.New(t, fakeKitKey, e2eIdentity)
	var out bytes.Buffer
	if err := runKeyLogin(context.Background(), &out, api.URL, fakeKitKey); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(daemon.DefaultConfigPath())
	o, err := kitcapture.OpenOutbox(daemon.KitOutboxDir(), e2eIdentity.TeamID)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Enqueue(kitcapture.KindSessions, []byte(`{"rows":[]}`)); err != nil {
		t.Fatal(err)
	}
	api.Revoke(fakeKitKey)
	t.Setenv("HEV_API_KEY", fakeKitKey)
	_, err = o.Drain(context.Background(), daemon.KitClient(&daemon.KitConfig{API: api.URL, Key: fakeKitKey}), nil)
	if !errors.Is(err, kitapi.ErrUnauthorized) || !strings.Contains(err.Error(), "HEV_API_KEY was refused") {
		t.Fatalf("unexpected refusal: %v", err)
	}
	refusal := err
	pending, err := o.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatal("outbox lost")
	}
	after, _ := os.ReadFile(daemon.DefaultConfigPath())
	if !bytes.Equal(before, after) {
		t.Fatal("401 changed stored config")
	}
	if strings.Contains(errString(refusal), fakeKitKey) {
		t.Fatal("key leaked")
	}
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestAPIKeyLoginPreservesEditDuringVerification(t *testing.T) {
	kitE2EHome(t)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := os.WriteFile(daemon.DefaultConfigPath(), []byte("unrelated = 'edited during verification'\n"), 0600); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, "{}")
	}))
	defer s.Close()
	var out bytes.Buffer
	if err := runKeyLogin(context.Background(), &out, s.URL, "synthetic-login-key"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(daemon.DefaultConfigPath())
	if !strings.Contains(string(raw), "edited during verification") {
		t.Fatal("lost concurrent config edit")
	}
}
