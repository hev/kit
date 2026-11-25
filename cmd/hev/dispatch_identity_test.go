//go:build !windows

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDispatchIdentityWorkspaceFlag(t *testing.T) {
	const workspace = "42d2ad7b-ad91-46c5-89eb-6e726cbbfb24"
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- r.Method + " " + r.URL.Path:
		default:
			t.Error("unexpected additional identity request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[],"next_after":null,"has_more":false}`)
	}))
	defer server.Close()

	home := t.TempDir()
	env := []string{
		"HEV_WORK_IDENTITY_ENDPOINT=" + server.URL,
		"HEV_WORK_IDENTITY_KEY=fixture-bearer",
	}
	_, stderr, code := runHevChild(t, home, env, "dispatch", "identity", "--workspace", workspace, "list")
	if code != 0 {
		t.Fatalf("identity list exited %d: %s", code, stderr)
	}
	select {
	case got := <-requests:
		want := "POST /v2/namespaces/work-" + workspace + "/work-identity/query"
		if got != want {
			t.Fatalf("request = %q, want %q", got, want)
		}
	default:
		t.Fatal("identity list made no request")
	}

	_, stderr, code = runHevChild(t, home, env, "dispatch", "identity", "--workspace=", "list")
	if code == 0 || !strings.Contains(stderr, "--workspace:") {
		t.Fatalf("empty identity workspace: exit %d, stderr %q", code, stderr)
	}
	select {
	case got := <-requests:
		t.Fatalf("empty identity workspace made a request: %s", got)
	default:
	}

	_, stderr, code = runHevChild(t, home, nil, "--workspace=", "workspace", "current")
	if code == 0 || !strings.Contains(stderr, "workspace name must not be empty") {
		t.Fatalf("empty root workspace: exit %d, stderr %q", code, stderr)
	}
}
