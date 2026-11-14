package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func signupTest(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	kitTestHome(t)
	t.Setenv("HEV_NO_HINTS", "")
	c := &cobra.Command{Use: "dispatch"}
	c.Flags().Bool("json", false, "")
	var out bytes.Buffer
	c.SetContext(context.Background())
	c.SetOut(&out)
	return c, &out
}
func readSignup(t *testing.T) signupState {
	t.Helper()
	raw, err := os.ReadFile(signupPath())
	if err != nil {
		t.Fatal(err)
	}
	var s signupState
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestSignupHints(t *testing.T) {
	c, out := signupTest(t)
	signupHint(c, "board")
	signupHint(c, "board")
	signupHint(c, "traces")
	if strings.Count(out.String(), "hev login") != 2 {
		t.Fatal(out.String())
	}
	updateSignup(func(s *signupState) { s.Days["board"] = "2000-01-01" })
	signupHint(c, "board")
	if strings.Count(out.String(), "hev login") != 3 {
		t.Fatal(out.String())
	}
	for _, mode := range []string{"json", "env", "config", "signed-in"} {
		t.Run(mode, func(t *testing.T) {
			c, out := signupTest(t)
			switch mode {
			case "json":
				c.Flags().Set("json", "true")
			case "env":
				t.Setenv("HEV_NO_HINTS", "1")
			case "config":
				os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("hints = false\n"), 0600)
			case "signed-in":
				os.WriteFile(os.Getenv("HEV_CONFIG"), []byte("[kit]\nkey = 'hk_test'\n"), 0600)
			}
			signupHint(c, "board")
			if out.Len() != 0 {
				t.Fatal(out.String())
			}
		})
	}
	// A buffer represents non-TTY human-formatted agent output, intentionally eligible.
}
func TestSignupEarnedFifthLaunch(t *testing.T) {
	c, out := signupTest(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "POST" || r.URL.Path != "/v1/codes/earned" {
			t.Error(r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 2 || body["dispatches"] != float64(5) || body["install_id"] == "" || body["install_id"] != readSignup(t).InstallID {
			t.Errorf("unexpected body: %v", body)
		}
		json.NewEncoder(w).Encode(earnedCode{"FREE_5", "campaign credit", time.Now().Add(time.Hour)})
	}))
	defer server.Close()
	t.Setenv(kitServiceEnv, server.URL)
	launch := func() { fetchEarned(context.Background(), out) }
	for i := 0; i < 4; i++ {
		signupDispatch(c, false, launch)
	}
	if requests != 0 {
		t.Fatal(requests)
	}
	signupDispatch(c, false, launch)
	signupDispatch(c, false, launch)
	if requests != 1 || strings.Count(out.String(), "Here's a code") != 1 {
		t.Fatal(requests, out.String())
	}
	s := readSignup(t)
	if s.Dispatches != 6 || !s.Finished || s.Earned.Code != "FREE_5" {
		t.Fatal(s)
	}
	out.Reset()
	showEarnedCode(c)
	if !strings.Contains(out.String(), "FREE_5") {
		t.Fatal(out.String())
	}
	updateSignup(func(s *signupState) { s.Earned.Expires = time.Now().Add(-time.Hour) })
	out.Reset()
	showEarnedCode(c)
	if out.Len() != 0 {
		t.Fatal(out.String())
	}
}
func TestSignupTimeoutAndRetry(t *testing.T) {
	c, out := signupTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	t.Setenv(kitServiceEnv, server.URL)
	for i := 0; i < 5; i++ {
		signupDispatch(c, false, func() {})
	}
	start := time.Now()
	fetchEarned(context.Background(), out)
	if elapsed := time.Since(start); elapsed > earnedTimeout+time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
	if out.Len() != 0 || readSignup(t).Finished {
		t.Fatal(out.String())
	}
	// Retry against an explicit campaign-off response, without rerunning a failed check.
	off := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer off.Close()
	t.Setenv(kitServiceEnv, off.URL)
	fetchEarned(context.Background(), out)
	if !readSignup(t).Finished {
		t.Fatal("204 must finish campaign")
	}
}
func TestSignupDispatchSuppressionStillCounts(t *testing.T) {
	for _, mode := range []string{"json", "env"} {
		t.Run(mode, func(t *testing.T) {
			c, out := signupTest(t)
			if mode == "json" {
				c.Flags().Set("json", "true")
			} else {
				t.Setenv("HEV_NO_HINTS", "1")
			}
			for i := 0; i < 6; i++ {
				signupDispatch(c, true, func() { t.Error("unexpected network helper") })
			}
			if readSignup(t).Dispatches != 6 || out.Len() != 0 {
				t.Fatal(out.String())
			}
		})
	}
}

func TestSignupBoardRawJSONAndReminderSuppression(t *testing.T) {
	for _, args := range [][]string{{"ls", "--json"}, {"search", "x", "--json=true"}, {"url"}, {"serve"}} {
		if boardHintOutput(args) {
			t.Fatal(args)
		}
	}
	if !boardHintOutput([]string{"search", "query"}) {
		t.Fatal("human search excluded")
	}
	c, out := signupTest(t)
	updateSignup(func(s *signupState) { s.Earned = &earnedCode{"CODE", "credit", time.Now().Add(time.Hour)} })
	c.Flags().Set("json", "true")
	showEarnedCode(c)
	if out.Len() != 0 {
		t.Fatal(out.String())
	}
	c.Flags().Set("json", "false")
	t.Setenv("HEV_NO_HINTS", "1")
	showEarnedCode(c)
	if out.Len() != 0 {
		t.Fatal(out.String())
	}
}

func TestSignupConcurrentHelpersOneRequest(t *testing.T) {
	c, out := signupTest(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(earnedCode{"ONCE", "credit", time.Now().Add(time.Hour)})
	}))
	defer server.Close()
	t.Setenv(kitServiceEnv, server.URL)
	for i := 0; i < 5; i++ {
		signupDispatch(c, false, func() {})
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); fetchEarned(context.Background(), out) }()
	}
	wg.Wait()
	if requests.Load() != 1 || strings.Count(out.String(), "Here's a code") != 1 {
		t.Fatal(requests.Load(), out.String())
	}
}

func TestSignupBoardHookJSON(t *testing.T) {
	_, out := signupTest(t)
	root := &cobra.Command{Use: "hev", PersistentPostRun: signupPostRun}
	board := &cobra.Command{Use: "board", DisableFlagParsing: true, Run: func(*cobra.Command, []string) {}}
	root.AddCommand(board)
	root.SetOut(out)
	root.SetArgs([]string{"board", "ls", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal(out.String())
	}
	root.SetArgs([]string{"board", "ls"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hev login") {
		t.Fatal(out.String())
	}
}
