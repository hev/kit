package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/local"
)

func socketServer(t *testing.T, ln net.Listener, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
}

// Docker, launchctl and gh are sandbox fakes. HTTP and PortFree use real
// sockets. The portFree seam starts the simulated service only AFTER the real
// bind check succeeds, standing in for Compose/launchd starting that service.
func TestUpIPv6OnlyListenersCoexistWithIPv4Readiness(t *testing.T) {
	onOS(t, "darwin")
	home, log := sandbox(t, "0")
	t.Setenv("FAKE_UNLOADED", "1")
	t.Setenv("TURBOPUFFER_API_KEY", "tpuf_test")
	var impostorHits, gatewayHits, readHits atomic.Int32
	ports := make([]int, 2)
	for i := range ports {
		ln, err := net.Listen("tcp6", "[::1]:0")
		if err != nil {
			t.Skipf("IPv6 loopback unavailable: %v", err)
		}
		addr := ln.Addr().(*net.TCPAddr)
		if addr.IP.To4() != nil || !addr.IP.Equal(net.IPv6loopback) {
			ln.Close()
			t.Fatalf("expected IPv6 loopback, got %v", addr)
		}
		ports[i] = addr.Port
		socketServer(t, ln, func(w http.ResponseWriter, r *http.Request) {
			impostorHits.Add(1)
			http.NotFound(w, r)
		})
		// tcp6 explicitly requests an IPv6-only socket. A simultaneous real
		// IPv4 bind below also proves it does not reserve the IPv4 address.
		t.Logf("IPv6-only impostor bound to %s (tcp6)", addr)
	}
	t.Setenv("HEV_LOCAL_PORT", strconv.Itoa(ports[0]))
	t.Setenv("HEV_LOCAL_SERVE_PORT", strconv.Itoa(ports[1]))
	writeConfig(t, home, fmt.Sprintf("[layer]\nendpoint = \"http://localhost:%d\"\nnamespace = \"mine\"\n[capture]\nscan_interval = \"1m\"\n", ports[0]))

	was := portFree
	t.Cleanup(func() { portFree = was })
	checks := 0
	portFree = func(port int) bool {
		free := local.PortFree(port)
		if !free {
			t.Fatalf("IPv6-only listener blocked IPv4 port %d", port)
		}
		checks++
		ln, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("IPv4 service bound concurrently to %s (tcp4)", ln.Addr())
		socketServer(t, ln, func(w http.ResponseWriter, r *http.Request) {
			if port == ports[0] {
				gatewayHits.Add(1)
				if r.URL.Path != "/health" && r.URL.Path != "/v2/namespaces/mine/query" {
					t.Errorf("unexpected gateway request %s", r.URL.Path)
				}
				fmt.Fprint(w, `{"status":"ok","version":"0.6.0-dev"}`)
			} else {
				readHits.Add(1)
				fmt.Fprint(w, "read side")
			}
		})
		return free
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := runUp(ctx, &out, false); err != nil {
		t.Fatalf("up: %v\n%s", err, &out)
	}
	if checks != 2 || gatewayHits.Load() < 2 || readHits.Load() == 0 || impostorHits.Load() != 0 {
		t.Fatalf("checks=%d gateway=%d read=%d impostor=%d", checks, gatewayHits.Load(), readHits.Load(), impostorHits.Load())
	}
	if !strings.Contains(out.String(), fmt.Sprintf("running on :%d", ports[0])) ||
		!strings.Contains(out.String(), fmt.Sprintf("http://127.0.0.1:%d", ports[1])) ||
		!strings.Contains(calls(t, log), "up --detach --wait") {
		t.Fatalf("missing success/compose evidence: %s\n%s", &out, calls(t, log))
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LayerEndpoint != fmt.Sprintf("http://127.0.0.1:%d", ports[0]) || cfg.LayerNamespace != "mine" || cfg.ScanInterval != time.Minute {
		t.Fatalf("persisted config lost local target or user settings: %+v", cfg)
	}
	if err := gatewayHealthy(layer.New(cfg.LayerEndpoint, cfg.LayerAPIKey, "", "")); err != nil {
		t.Fatalf("persisted endpoint readiness: %v", err)
	}
	before, err := os.Stat(cfg.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if w, err := daemon.WriteLocalConfig(cfg.Local, "tpuf_test"); err != nil || w.Changed {
		t.Fatalf("repeat config write: %+v err=%v", w, err)
	}
	after, err := os.Stat(cfg.ConfigPath)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("idempotent config rewritten: %v", err)
	}
	if impostorHits.Load() != 0 {
		t.Fatal("persisted endpoint contacted IPv6 impostor")
	}
	t.Log(out.String())
}

func TestUpIPv4WildcardConflictStopsBeforeCompose(t *testing.T) {
	onOS(t, "darwin")
	_, log := sandbox(t, "0")
	t.Setenv("TURBOPUFFER_API_KEY", "tpuf_test")
	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)
	if addr.IP.To4() == nil || !addr.IP.IsUnspecified() {
		t.Fatalf("expected IPv4 wildcard, got %v", addr)
	}
	// Verify that the actual listener accepts an IPv4 loopback connection.
	conn, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", addr.Port), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	t.Setenv("HEV_LOCAL_PORT", strconv.Itoa(addr.Port))
	var out bytes.Buffer
	err = runUp(context.Background(), &out, false)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("port %d is in use", addr.Port)) || !strings.Contains(err.Error(), "HEV_LOCAL_PORT") {
		t.Fatalf("conflict error = %v", err)
	}
	if got := calls(t, log); strings.Contains(got, "up --detach") || strings.Contains(got, "launchctl") {
		t.Fatalf("started services despite conflict:\n%s", got)
	}
	t.Logf("verified tcp4 wildcard %s serves IPv4 loopback; up error: %v", addr, err)
}
