package dispatch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBoatHTTPProvider(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Error("missing bearer")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/sandboxes":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["from"] != "template" || body["type"] != "small" || body["ttlSeconds"] != float64(900) || body["environment"] != "demo" {
				t.Errorf("create: %#v", body)
			}
			labels := body["labels"].(map[string]any)
			if labels["hev.dispatch"] != "cloud" || labels["hev.order"] != "one" {
				t.Error(labels)
			}
			fmt.Fprint(w, `{"sandbox":{"id":"bx_test","state":"ready"}}`)
		case r.Method == "GET" && r.URL.Path == "/sandboxes":
			if r.URL.Query().Get("label") != "hev.dispatch=cloud" {
				t.Error(r.URL.RawQuery)
			}
			if r.URL.Query().Get("cursor") == "" {
				fmt.Fprint(w, `{"sandboxes":[{"id":"bx_test","labels":{"hev.order":"one"}}],"pageInfo":{"hasMore":true,"nextCursor":"page2"}}`)
			} else {
				fmt.Fprint(w, `{"sandboxes":[],"pageInfo":{"hasMore":false}}`)
			}
		case r.URL.Path == "/sandboxes/bx_test/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "echo test" {
				t.Error(body)
			}
			fmt.Fprint(w, `{"stdout":"test\n","exitCode":0}`)
		case r.URL.Path == "/sandboxes/bx_test/files":
			if r.URL.Query().Get("path") != "/tmp/file" || r.URL.Query().Get("encoding") != "base64" {
				t.Error(r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"content":"dGVzdA=="}`)
		default:
			fmt.Fprint(w, `{"sandbox":{"id":"bx_test","state":"ready"}}`)
		}
	}))
	defer srv.Close()
	b := &Boat{endpoint: srv.URL, key: "synthetic-key", client: srv.Client()}
	ctx := context.Background()
	m, err := b.Create(ctx, MachineSpec{Name: "d-one", Host: "cloud", From: "template", Size: "small", Environment: "demo", AutoStop: 15 * time.Minute})
	if err != nil || m.ID != "bx_test" {
		t.Fatal(m, err)
	}
	if err = b.WaitReady(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	list, err := b.List(ctx, "cloud")
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	out, err := b.Exec(ctx, m.ID, "echo test")
	if err != nil || out != "test\n" {
		t.Fatal(out, err)
	}
	data, err := b.Copy(ctx, m.ID, "/tmp/file")
	if err != nil || string(data) != "test" {
		t.Fatal(string(data), err)
	}
	if err = b.Stop(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = b.Resume(ctx, m.ID, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = b.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 10 {
		t.Fatalf("calls=%v", calls)
	}
}
func TestBoatHTTPFailuresDoNotExposeSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"code":"forbidden","message":"synthetic-secret"}`)
	}))
	defer srv.Close()
	b := &Boat{endpoint: srv.URL, key: "synthetic-secret", client: srv.Client()}
	_, err := b.List(context.Background(), "cloud")
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal(err)
	}
}
func TestBoatCommandRejectsPartialOutput(t *testing.T) {
	for _, body := range []string{`{"exitCode":1}`, `{"exitCode":0,"stdoutTruncated":true}`, `{"exitCode":0,"timedOut":true}`, `{"exitCode":null}`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer srv.Close()
			b := &Boat{endpoint: srv.URL, client: srv.Client()}
			if _, err := b.Exec(context.Background(), "bx_test", "test"); err == nil {
				t.Fatal("accepted failed command")
			}
		})
	}
}
func TestBoatConfigAndEnvironmentOnlyKey(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"valid", "", true}, {"key-in-config", "api_key = 'forbidden'", false}, {"missing-template", "from = ''", false}, {"workspace", "workspace = '/tmp'", false}, {"bad-size", "size = 'huge'", false}, {"bad-env", "api_key_env = 'bad-name'", false}, {"region", "region = 'eu'", false}, {"negative-stop", "auto_stop_minutes = -1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "[hosts.cloud]\ntype = 'boat'\ncap = 3\n"
			if tc.name != "missing-template" {
				text += "from = 'template'\n"
			}
			text += tc.extra
			path := filepath.Join(t.TempDir(), "dispatch.toml")
			_ = os.WriteFile(path, []byte(text), 0600)
			c, err := Load(path)
			if (err == nil) != tc.valid {
				t.Fatalf("config err=%v", err)
			}
			if tc.valid {
				if c.Hosts["cloud"].Name != "cloud" || c.Hosts["cloud"].APIKeyEnv != "BOAT_API_KEY" {
					t.Fatal(c)
				}
			}
		})
	}
	t.Setenv("BOAT_API_KEY", "")
	if _, err := NewBoat(Host{}); err == nil {
		t.Fatal("empty key accepted")
	}
	t.Setenv("CUSTOM_BOAT_TEST_KEY", "synthetic-key")
	b, err := NewBoat(Host{APIKeyEnv: "CUSTOM_BOAT_TEST_KEY"})
	if err != nil || b.key != "synthetic-key" {
		t.Fatal(err)
	}
}

type fakeEphemeral struct {
	machines                  []Machine
	created, deleted, stopped int
	files                     map[string]string
	archive                   []byte
	copyErr                   error
	commands                  []string
}

func (f *fakeEphemeral) Create(context.Context, MachineSpec) (Machine, error) {
	f.created++
	return Machine{ID: "new"}, nil
}
func (f *fakeEphemeral) List(context.Context, string) ([]Machine, error)     { return f.machines, nil }
func (f *fakeEphemeral) WaitReady(context.Context, string) error             { return nil }
func (f *fakeEphemeral) Resume(context.Context, string, time.Duration) error { return nil }
func (f *fakeEphemeral) Stop(context.Context, string) error                  { f.stopped++; return nil }
func (f *fakeEphemeral) Delete(context.Context, string) error                { f.deleted++; return nil }
func (f *fakeEphemeral) Copy(context.Context, string, string) ([]byte, error) {
	return f.archive, f.copyErr
}
func (f *fakeEphemeral) Exec(_ context.Context, _ string, cmd string) (string, error) {
	f.commands = append(f.commands, cmd)
	if strings.Contains(cmd, "tar --exclude=wt") {
		return "/tmp/hev-record-one.test/part-aa\n", nil
	}
	if strings.Contains(cmd, "hev dispatch rpc") {
		encoded := strings.Split(cmd, "'")[1]
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", err
		}
		var q Request
		if err = json.Unmarshal(data, &q); err != nil {
			return "", err
		}
		var r Response
		if q.Action == "show" {
			r.Files = f.files
		} else if q.Action == "list" {
			r.Rows = []Row{{Meta: Meta{Order: Order{Name: "one"}}, State: "done"}}
		} else {
			r.Message = "ok"
		}
		data, _ = json.Marshal(r)
		return string(data), nil
	}
	return "", nil
}
func boatArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(data))
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}
func withFakeBoat(t *testing.T, f *fakeEphemeral) {
	t.Helper()
	old := ephemeralProvider
	ephemeralProvider = func(Host) (EphemeralProvider, error) { return f, nil }
	t.Cleanup(func() { ephemeralProvider = old })
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(Root(), 0700); err != nil {
		t.Fatal(err)
	}
}
func TestBoatCapAdmission(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "existing", State: "provisioning", Labels: map[string]string{"hev.order": "other"}}}}
	withFakeBoat(t, f)
	_, err := boatRPC(context.Background(), Host{Name: "cloud", Cap: 1}, Request{Action: "start", Name: "one", Meta: Meta{Order: Order{Repo: "example/repo", Validation: "none"}}})
	if err == nil || !strings.Contains(err.Error(), "host full") || f.created != 0 {
		t.Fatal(err, f.created)
	}
}
func TestBoatFetchBackNotifyAndClean(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("notification fixture uses sh")
	}
	files := map[string]string{"meta.json": `{"name":"one","host":"cloud","ephemeral":true}`, "result.json": `{"exit_code":0,"end":"2026-01-01T00:00:00Z"}`, "report.md": "kept report", "log": "kept log", "evidence/check.txt": "kept evidence"}
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: files, archive: boatArchive(t, files)}
	withFakeBoat(t, f)
	marker := filepath.Join(t.TempDir(), "notifications")
	h := Host{Type: "boat", Name: "cloud", Notify: []string{"sh", "-c", "echo notified >> " + Quote(marker)}}
	for i := 0; i < 2; i++ {
		if _, err := boatRPC(context.Background(), h, Request{Action: "show", Name: "one"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "notified\n" {
		t.Fatalf("notify=%q err=%v", data, err)
	}
	if _, err := boatRPC(context.Background(), h, Request{Action: "clean", Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if f.deleted != 1 {
		t.Fatal("not deleted")
	}
	for name, want := range files {
		data, err := os.ReadFile(filepath.Join(Root(), "one", name))
		if err != nil || string(data) != want {
			t.Fatal(name, string(data), err)
		}
	}
}
func TestBoatCleanRefusesLostRecord(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: map[string]string{"result.json": `{"end":"2026-01-01T00:00:00Z"}`}, copyErr: fmt.Errorf("copy failed")}
	withFakeBoat(t, f)
	if _, err := boatRPC(context.Background(), Host{Name: "cloud"}, Request{Action: "clean", Name: "one"}); err == nil || f.deleted != 0 {
		t.Fatal(err, f.deleted)
	}
}
func TestBoatArchiveRejectsTraversal(t *testing.T) {
	if err := extractBoatRecord(t.TempDir(), boatArchive(t, map[string]string{"../escaped": "bad"})); err == nil {
		t.Fatal("accepted traversal")
	}
}

func TestBoatArchiveShellManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix VM shell")
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "report.md"), []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-c", boatArchiveCommand(source, filepath.Join(t.TempDir(), "archive.XXXXXX"))).Output()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Fields(string(out))
	if len(parts) != 1 {
		t.Fatalf("manifest=%q", out)
	}
	data, err := os.ReadFile(parts[0])
	if err != nil {
		t.Fatalf("manifest %q: %v", out, err)
	}
	target := t.TempDir()
	if err = extractBoatRecord(target, data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(target, "report.md"))
	if err != nil || string(got) != "report" {
		t.Fatal(string(got), err)
	}
}

func TestBoatStartUsesUnixRPCAndLocalWatcher(t *testing.T) {
	f := &fakeEphemeral{}
	withFakeBoat(t, f)
	old := boatWatchLauncher
	t.Cleanup(func() { boatWatchLauncher = old })
	watched := ""
	boatWatchLauncher = func(h Host, name string) error { watched = h.Name + "/" + name; return nil }
	h := Host{Type: "boat", Name: "cloud", Cap: 3, Size: "small", From: "template"}
	q := Request{Action: "start", Name: "one", Meta: Meta{Order: Order{Name: "one", Host: "cloud", Repo: "example/repo", Validation: "none", Timeout: 10}}}
	if _, err := boatRPC(context.Background(), h, q); err != nil {
		t.Fatal(err)
	}
	if f.created != 1 || watched != "cloud/one" {
		t.Fatal(f.created, watched)
	}
	var sent Request
	for _, cmd := range f.commands {
		if strings.Contains(cmd, "git clone") && !strings.Contains(cmd, "https://github.com/example/repo.git") {
			t.Fatal(cmd)
		}
		if strings.Contains(cmd, "hev dispatch rpc") {
			encoded := strings.Split(cmd, "'")[1]
			data, _ := base64.StdEncoding.DecodeString(encoded)
			_ = json.Unmarshal(data, &sent)
		}
	}
	if !sent.Meta.Ephemeral || sent.Cap != 1 || sent.Meta.ConfigPath != "/home/user/.config/hev/boat-dispatch.toml" {
		t.Fatal(sent)
	}
}
func TestBoatListRecoversArchivedRecordWithoutLocalRegistry(t *testing.T) {
	files := map[string]string{"meta.json": `{"name":"one","host":"cloud","ephemeral":true}`, "result.json": `{"exit_code":0,"end":"2026-01-01T00:00:00Z"}`}
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "archived", Labels: map[string]string{"hev.order": "one"}}}, files: files, archive: boatArchive(t, files)}
	withFakeBoat(t, f)
	r, err := boatRPC(context.Background(), Host{Name: "cloud", Cap: 3}, Request{Action: "list"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 1 || r.Rows[0].Name != "one" || r.Rows[0].State != "done" || f.stopped != 1 {
		t.Fatal(r, f.stopped)
	}
}
func TestBoatResumeCapAdmission(t *testing.T) {
	f := &fakeEphemeral{machines: []Machine{{ID: "other", State: "running"}, {ID: "vm", State: "archived", Labels: map[string]string{"hev.order": "one"}}}}
	withFakeBoat(t, f)
	_, err := boatRPC(context.Background(), Host{Name: "cloud", Cap: 1}, Request{Action: "rework", Name: "one", Note: "again"})
	if err == nil || !strings.Contains(err.Error(), "host full") {
		t.Fatal(err)
	}
}

func TestBoatNotifyFailureRetriesFromArchivedCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("notification fixture uses sh")
	}
	files := map[string]string{"meta.json": `{"name":"one","host":"cloud","ephemeral":true}`, "result.json": `{"exit_code":0,"end":"2026-01-01T00:00:00Z"}`}
	f := &fakeEphemeral{machines: []Machine{{ID: "vm", State: "ready", Labels: map[string]string{"hev.order": "one"}}}, files: files, archive: boatArchive(t, files)}
	withFakeBoat(t, f)
	h := Host{Name: "cloud", Cap: 3, Notify: []string{"sh", "-c", "exit 1"}}
	if _, err := boatRPC(context.Background(), h, Request{Action: "show", Name: "one"}); err == nil {
		t.Fatal("notification failure ignored")
	}
	f.machines[0].State = "archived"
	marker := filepath.Join(t.TempDir(), "notified")
	h.Notify = []string{"sh", "-c", "echo notified >> " + Quote(marker)}
	if _, err := boatRPC(context.Background(), h, Request{Action: "list"}); err != nil {
		t.Fatal(err)
	}
	if _, err := boatRPC(context.Background(), h, Request{Action: "show", Name: "one"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "notified\n" {
		t.Fatal(string(data), err)
	}
}
