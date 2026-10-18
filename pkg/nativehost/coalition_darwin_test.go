//go:build darwin

package nativehost

import (
	"bytes"
	"encoding/json"
	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"strings"
	"testing"
)

func coalitionFixtureBirth(t *testing.T) n.Birth {
	t.Helper()
	build, e := unix.Sysctl("kern.osversion")
	if e != nil || build != "25G83" || runtime.GOARCH != "arm64" {
		t.Skip("private ABI requires the exact native-tested build")
	}
	b, e := n.ProcessBirth(os.Getpid())
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestDarwinCoalitionRequiresOriginalLiveBirthAndCID(t *testing.T) {
	b := coalitionFixtureBirth(t)
	got, e := ObserveResourceCoalition(b, 0)
	if e != nil {
		t.Fatal(e)
	}
	if got.Version != CoalitionVersion || got.HostBirth != b || got.ResourceCID == 0 || got.TasksStarted <= got.TasksExited || got.OSBuild != "25G83" || got.ObservedAt.IsZero() {
		t.Fatal("invalid sample", got)
	}
	if _, e = ObserveResourceCoalition(b, got.ResourceCID); e != nil {
		t.Fatal(e)
	}
	if _, e = ObserveResourceCoalition(b, got.ResourceCID+1); e == nil {
		t.Fatal("different coalition accepted")
	}
	wrong := b
	wrong.Start += "-different"
	if _, e = ObserveResourceCoalition(wrong, got.ResourceCID); e == nil {
		t.Fatal("different original birth accepted")
	}
}
func TestDarwinCoalitionCodecIsStrictAndObservationOnly(t *testing.T) {
	b := coalitionFixtureBirth(t)
	raw, _ := json.Marshal(map[string]any{"version": CoalitionVersion, "host_birth": b, "resource_cid": 0})
	good := string(raw)
	var out bytes.Buffer
	if e := WriteCoalitionObservation(strings.NewReader(good), &out); e != nil {
		t.Fatal(e)
	}
	var result map[string]any
	if json.Unmarshal(out.Bytes(), &result) != nil || len(result) != 7 {
		t.Fatal("unexpected observation fields")
	}
	for _, bad := range []string{strings.Replace(good, `"version"`, `"VERSION"`, 1), strings.Replace(good, `"pid"`, `"PID"`, 1), strings.Replace(good, `"resource_cid":0`, `"resource_cid":null`, 1), strings.Replace(good, `"resource_cid":0`, `"resource_cid":0,"resource_cid":0`, 1), strings.Replace(good, `"version":`, `"argv":[],"version":`, 1), good + ` {}`, good + strings.Repeat(" ", maxFrame)} {
		out.Reset()
		if e := WriteCoalitionObservation(strings.NewReader(bad), &out); e == nil || out.Len() != 0 {
			t.Fatalf("ambiguous observation input accepted: %.100s", bad)
		}
	}
}
