package main

import (
	"bytes"
	"context"
	"fmt"
	boardcli "github.com/hev/kit/internal/board/cli"
	"github.com/hev/kit/pkg/kitapi"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDeltaLoginSearchBoard(t *testing.T) {
	for _, status := range []int{402, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			d := t.TempDir()
			t.Setenv("HOME", d)
			t.Setenv("HEV_CONFIG", d+"/config.toml")
			t.Setenv("HEV_API_KEY", "")
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(status)
				fmt.Fprintf(w, `{"error":%q}`, "delta-active delta-stored")
			}))
			defer s.Close()
			raw := []byte("[kit]\napi='" + s.URL + "'\nkey='delta-stored'\n")
			os.WriteFile(d+"/config.toml", raw, 0600)
			var out bytes.Buffer
			e := runKeyLogin(context.Background(), &out, s.URL, "delta-active")
			after, _ := os.ReadFile(d + "/config.toml")
			if e == nil || string(after) != string(raw) || isUnauthorized(e) {
				t.Fatal("login mutated config or refused key")
			}
			checkDeltaGuidance(t, e, status)
			c := kitapi.New(s.URL, "delta-active", "delta-stored")
			c.Retries = 0
			e = runKitSearch(context.Background(), &out, c, []string{"probe"}, 1, true)
			if e == nil || isUnauthorized(e) {
				t.Fatal("search misclassified")
			}
			checkDeltaGuidance(t, e, status)
			boardcli.KitAPI = s.URL
			boardcli.KitKey = "delta-active"
			boardcli.HostedClient = c
			defer func() { boardcli.KitAPI = ""; boardcli.KitKey = ""; boardcli.HostedClient = nil }()
			e = boardcli.Run([]string{"search", "probe"})
			if e == nil || isUnauthorized(e) {
				t.Fatal("board misclassified")
			}
			checkDeltaGuidance(t, e, status)
			if strings.Contains(out.String(), "delta-active") || strings.Contains(out.String(), "delta-stored") {
				t.Fatal("output leaked")
			}
		})
	}
}

func checkDeltaGuidance(t *testing.T, e error, status int) {
	t.Helper()
	s := e.Error()
	if status == 402 {
		if strings.Contains(s, "reads still work") || !strings.Contains(s, "/billing") || !strings.Contains(s, "code") {
			t.Error("missing accurate billing guidance")
		}
	} else if !strings.Contains(s, "not refused") || !strings.Contains(s, "try again") || !strings.Contains(s, "1s") {
		t.Error("missing temporary outage and retry guidance")
	}
}
