package dispatch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/version"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ephemeralProvider = func(h Host) (EphemeralProvider, error) { return NewBoat(h) }
var boatWatchLauncher = launchBoatWatch
var repoSlug = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func boatRepo(repo string) (string, error) {
	if repoSlug.MatchString(repo) {
		return "https://github.com/" + repo + ".git", nil
	}
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "ssh://") || strings.HasPrefix(repo, "git@") {
		return repo, nil
	}
	return "", fmt.Errorf("Boat repo must be a git URL or owner/repo")
}
func boatTTL(h Host, o Order) time.Duration {
	if h.AutoStopMinutes > 0 {
		return time.Duration(h.AutoStopMinutes) * time.Minute
	}
	return time.Duration(o.Timeout+10) * time.Minute
}
func providerRPC(ctx context.Context, p EphemeralProvider, id string, q Request) (Response, error) {
	var r Response
	data, err := json.Marshal(q)
	if err != nil {
		return r, err
	}
	out, err := p.Exec(ctx, id, "printf %s "+Quote(base64.StdEncoding.EncodeToString(data))+" | base64 -d | hev dispatch rpc")
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(out), &r); err != nil {
		return r, fmt.Errorf("invalid Boat RPC response: %w", err)
	}
	if r.Error != "" {
		return r, fmt.Errorf("%s", r.Error)
	}
	return r, nil
}
func boatRPC(ctx context.Context, h Host, q Request) (Response, error) {
	r := Response{Version: version.Version}
	if err := os.MkdirAll(Root(), 0700); err != nil {
		return r, err
	}
	p, err := ephemeralProvider(h)
	if err != nil {
		return r, err
	}
	machines, err := p.List(ctx, h.Name)
	if err != nil {
		return r, err
	}
	if q.Action == "list" {
		for _, m := range machines {
			name := m.Labels["hev.order"]
			if !ValidName(name) {
				return r, fmt.Errorf("invalid Boat order label")
			}
			if m.live() {
				r.Running++
			}
			var rows Response
			switch m.State {
			case "ready", "idle", "running":
				rows, err = providerRPC(ctx, p, m.ID, Request{Action: "list"})
				if err != nil {
					return r, err
				}
				for _, row := range rows.Rows {
					if row.Name == name && !rowIsActive(row) {
						if _, err = fetchBoatRecord(ctx, p, m.ID, h, name); err != nil {
							return r, err
						}
						if err = p.Stop(ctx, m.ID); err != nil {
							return r, err
						}
					}
				}
			case "archived":
				if files, e := cachedBoatFiles(name, m.ID, h.Name); e == nil {
					if err = notifyBoatRecord(ctx, h, name, files); err != nil {
						return r, err
					}
					rows = boatCachedRows(name)
				}
				if len(rows.Rows) == 0 {
					if err = resumeBoat(ctx, p, h, m.ID, 10*time.Minute); err != nil {
						return r, err
					}
					_, fetchErr := fetchBoatRecord(ctx, p, m.ID, h, name)
					stopErr := p.Stop(ctx, m.ID)
					if fetchErr != nil {
						return r, fetchErr
					}
					if stopErr != nil {
						return r, stopErr
					}
					rows = boatCachedRows(name)
				}
			default:
				rows = Response{Rows: []Row{{Meta: Meta{Order: Order{Name: name, Host: h.Name}}, State: m.State}}}
			}
			r.Rows = append(r.Rows, rows.Rows...)
			r.Names = append(r.Names, name)
		}
		return r, nil
	}
	if !ValidName(q.Name) {
		return r, fmt.Errorf("invalid name")
	}
	var machine Machine
	for _, m := range machines {
		if m.Labels["hev.order"] == q.Name {
			if machine.ID != "" {
				return r, fmt.Errorf("ambiguous Boat order")
			}
			machine = m
		}
	}
	if q.Action == "start" {
		repo, err := boatRepo(q.Meta.Repo)
		if err != nil {
			return r, err
		}
		if q.Meta.Validation != "none" {
			return r, fmt.Errorf("validation on Boat is unsupported")
		}
		// Serialize admission among processes on this dispatcher. The provider list
		// remains authoritative; independent dispatchers need separate host labels.
		unlock, err := lockRoot()
		if err != nil {
			return r, err
		}
		defer unlock()
		machines, err = p.List(ctx, h.Name)
		if err != nil {
			return r, err
		}
		live := 0
		for _, m := range machines {
			if m.Labels["hev.order"] == q.Name {
				return r, fmt.Errorf("Boat name already exists")
			}
			if m.live() {
				live++
			}
		}
		if err = CheckCap(live, h.Cap); err != nil {
			return r, err
		}
		if boatTTL(h, q.Meta.Order) > 30*24*time.Hour {
			return r, fmt.Errorf("Boat auto-stop must be at most 30 days")
		}
		machine, err = p.Create(ctx, MachineSpec{Name: "d-" + q.Name, Host: h.Name, From: h.From, Size: h.Size, Region: h.Region, Environment: h.Environment, AutoStop: boatTTL(h, q.Meta.Order)})
		if err != nil {
			return r, err
		}
		if err = p.WaitReady(ctx, machine.ID); err != nil {
			return r, err
		}
		// The generated target config contains no credentials and no dispatcher
		// notification command. Boat environment secrets supply harness/capture auth.
		targetConfig := fmt.Sprintf("[hosts.%s]\nlocal = true\ncap = 1\nnamespace = %q\n", q.Meta.Host, h.Namespace)
		config64 := base64.StdEncoding.EncodeToString([]byte(targetConfig))
		source := "/home/user/dispatch-source/" + q.Name
		_, err = p.Exec(ctx, machine.ID, "mkdir -p /home/user/.config/hev /home/user/dispatch-source && printf %s "+Quote(config64)+" | base64 -d > /home/user/.config/hev/boat-dispatch.toml && git clone -- "+Quote(repo)+" "+Quote(source))
		if err != nil {
			return r, err
		}
		if err = activateBoatCapture(ctx, p, machine.ID); err != nil {
			return r, err
		}
		q.Meta.Ephemeral = true
		q.Meta.ConfigPath = "/home/user/.config/hev/boat-dispatch.toml"
		q.Cap = 1
		r, err = providerRPC(ctx, p, machine.ID, q)
		if err != nil {
			return r, err
		}
		if err = boatWatchLauncher(h, q.Name); err != nil {
			return r, fmt.Errorf("run started but local Boat watcher failed: %w; fetch with show/ls", err)
		}
		return r, nil
	}
	if machine.ID == "" {
		return r, fmt.Errorf("Boat order not found")
	}
	if q.Action == "validate" {
		return r, fmt.Errorf("validation on Boat is unsupported")
	}
	if q.Action == "stop" && machine.State == "archived" {
		return Response{Message: "Boat VM already stopped"}, nil
	}
	if q.Action == "clean" && machine.State == "archived" {
		if files, e := cachedBoatFiles(q.Name, machine.ID, h.Name); e == nil {
			if err = notifyBoatRecord(ctx, h, q.Name, files); err != nil {
				return r, err
			}
			if err = p.Delete(ctx, machine.ID); err != nil {
				return r, err
			}
			return Response{Message: "deleted Boat VM; kept ~/dispatch/" + q.Name}, nil
		}
	}
	if !machine.live() {
		if q.Action == "show" {
			if files, e := cachedBoatFiles(q.Name, machine.ID, h.Name); e == nil {
				return Response{Files: files}, notifyBoatRecord(ctx, h, q.Name, files)
			}
		}
		if err = resumeBoat(ctx, p, h, machine.ID, boatTTL(h, Order{Timeout: 120})); err != nil {
			return r, err
		}
	}
	if q.Action == "rework" || q.Action == "say" {
		if err = activateBoatCapture(ctx, p, machine.ID); err != nil {
			return r, err
		}
	}
	if q.Action == "clean" {
		if _, err = fetchBoatRecord(ctx, p, machine.ID, h, q.Name); err != nil {
			return r, fmt.Errorf("refusing deletion before record fetch: %w", err)
		}
		if err = p.Delete(ctx, machine.ID); err != nil {
			return r, err
		}
		return Response{Message: "deleted Boat VM; kept ~/dispatch/" + q.Name}, nil
	}
	if q.Action == "rework" || q.Action == "say" {
		// A previous terminal record cannot authorize deletion after rework.
		_ = os.Remove(filepath.Join(Root(), q.Name, "boat.json"))
	}
	r, err = providerRPC(ctx, p, machine.ID, q)
	if err != nil {
		return r, err
	}
	switch q.Action {
	case "show":
		if r.Files["result.json"] != "" {
			_, err = fetchBoatRecord(ctx, p, machine.ID, h, q.Name)
		}
	case "stop":
		for {
			show, e := providerRPC(ctx, p, machine.ID, Request{Action: "show", Name: q.Name})
			if e != nil {
				return r, e
			}
			if show.Files["result.json"] != "" {
				break
			}
			select {
			case <-ctx.Done():
				return r, ctx.Err()
			case <-time.After(time.Second):
			}
		}
		if _, err = fetchBoatRecord(ctx, p, machine.ID, h, q.Name); err == nil {
			err = p.Stop(ctx, machine.ID)
		}
	case "rework", "say":
		if err = boatWatchLauncher(h, q.Name); err != nil {
			return r, err
		}
	}
	return r, err
}
func rowIsActive(row Row) bool {
	return row.State == "running" || row.State == "launching" || row.State == "pending" || row.State == "launch-unconfirmed"
}
func boatCachedRows(name string) Response {
	var m Meta
	var result Result
	dir := filepath.Join(Root(), name)
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
		return Response{}
	}
	var rp *Result
	if readJSON(filepath.Join(dir, "result.json"), &result) == nil {
		rp = &result
	}
	return Response{Rows: []Row{{Meta: m, State: State(rp, false), PR: result.PR, Trace: result.Session, Validation: result.Validation}}}
}

type boatCache struct{ ID, Host string }

func cachedBoatFiles(name, id, host string) (map[string]string, error) {
	dir := filepath.Join(Root(), name)
	var cache boatCache
	if err := readJSON(filepath.Join(dir, "boat.json"), &cache); err != nil {
		return nil, err
	}
	if cache.ID != id || cache.Host != host {
		return nil, fmt.Errorf("Boat cache belongs to another VM or host")
	}
	if _, err := os.Stat(filepath.Join(dir, "boat-record.tar.gz")); err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, f := range []string{"meta.json", "order.md", "status.md", "report.md", "result.json", "inbox.md", "validation.json", "verdict.json"} {
		data, _ := os.ReadFile(filepath.Join(dir, f))
		files[f] = string(data)
	}
	return files, nil
}
func fetchBoatRecord(ctx context.Context, p EphemeralProvider, id string, h Host, name string) (Response, error) {
	show, err := providerRPC(ctx, p, id, Request{Action: "show", Name: name})
	if err != nil {
		return show, err
	}
	var terminal Result
	if err = json.Unmarshal([]byte(show.Files["result.json"]), &terminal); err != nil || terminal.End.IsZero() || terminal.LaunchUnconfirmed {
		return show, fmt.Errorf("no confirmed terminal result; stop before clean")
	}

	// Chunk the archive below the Boat files API's 5 MiB limit. Include logs,
	// rework history and evidence; retain the worktree separately only in the VM.
	manifest, err := p.Exec(ctx, id, boatArchiveCommand("/home/user/dispatch/"+name, "/tmp/hev-record-"+name+".XXXXXX"))
	if err != nil {
		return show, err
	}
	var archive []byte
	for _, path := range strings.Fields(manifest) {
		if !strings.HasPrefix(path, "/tmp/hev-record-"+name+".") || filepath.Clean(path) != path || !strings.HasPrefix(filepath.Base(path), "part-") || filepath.Dir(filepath.Dir(path)) != "/tmp" {
			return show, fmt.Errorf("invalid archive part")
		}
		data, e := p.Copy(ctx, id, path)
		if e != nil {
			return show, e
		}
		archive = append(archive, data...)
	}
	if parts := strings.Fields(manifest); len(parts) > 0 {
		_, _ = p.Exec(ctx, id, "rm -rf -- "+Quote(filepath.Dir(parts[0])))
	}
	if len(archive) == 0 {
		return show, fmt.Errorf("empty Boat record")
	}
	dir := filepath.Join(Root(), name)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return show, err
	}
	var previousMeta Meta
	if readJSON(filepath.Join(dir, "meta.json"), &previousMeta) == nil && (!previousMeta.Ephemeral || previousMeta.Host != h.Name) {
		return show, fmt.Errorf("local dispatch record conflicts with Boat run %s", name)
	}
	if err = os.Remove(filepath.Join(dir, "boat.json")); err != nil && !os.IsNotExist(err) {
		return show, err
	}
	if err = extractBoatRecord(dir, archive); err != nil {
		return show, err
	}
	if err = os.WriteFile(filepath.Join(dir, "boat-record.tar.gz"), archive, 0600); err != nil {
		return show, err
	}
	if err = WriteJSON(filepath.Join(dir, "boat.json"), boatCache{ID: id, Host: h.Name}); err != nil {
		return show, err
	}
	return show, notifyBoatRecord(ctx, h, name, show.Files)
}
func notifyBoatRecord(ctx context.Context, h Host, name string, files map[string]string) error {
	var err error
	var m Meta
	var result Result
	if err = json.Unmarshal([]byte(files["meta.json"]), &m); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(files["result.json"]), &result); err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(files["result.json"])))
	// Notification is once per fetched result revision; failures remain retryable.
	unlock, err := lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	marker := filepath.Join(Root(), ".boat-notified-"+name)
	previous, _ := os.ReadFile(marker)
	if string(previous) != digest {
		if err = Notify(ctx, h.Notify, m, result); err != nil {
			return err
		}
		err = os.WriteFile(marker, []byte(digest), 0600)
	}
	return err
}
func extractBoatRecord(dir string, data []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || strings.Contains(name, "\\") {
			return fmt.Errorf("unsafe Boat record path")
		}
		for check := filepath.Join(dir, name); check != dir; check = filepath.Dir(check) {
			if st, e := os.Lstat(check); e == nil && st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in Boat record destination")
			}
		}
		if hdr.Typeflag == tar.TypeDir {
			if err = os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
				return err
			}
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsupported Boat record entry")
		}
		path := filepath.Join(dir, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}

		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
func launchBoatWatch(h Host, name string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	config := os.Getenv("HEV_DISPATCH_CONFIG")
	if h.DispatchConfig != "" {
		config = h.DispatchConfig
	}
	c := exec.Command(exe, "dispatch", "boat-watch", name, "--host", h.Name, "--config", config)
	log, err := os.OpenFile(filepath.Join(Root(), ".boat-watch-"+name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	c.Stdout, c.Stderr = log, log
	setupChild(c)
	if err = c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// WatchBoat runs on the dispatcher, so a VM never receives its provider key or
// notification command. Auto-stop remains a backstop if the dispatcher exits.
func WatchBoat(ctx context.Context, h Host, name string) error {
	p, err := ephemeralProvider(h)
	if err != nil {
		return err
	}
	for {
		machines, err := p.List(ctx, h.Name)
		if err != nil {
			return err
		}
		found := false
		for _, m := range machines {
			if m.Labels["hev.order"] != name {
				continue
			}
			found = true
			if !m.live() || m.State == "archiving" {
				return nil
			}
			r, err := providerRPC(ctx, p, m.ID, Request{Action: "show", Name: name})
			if err != nil {
				return err
			}
			if r.Files["result.json"] != "" {
				// Give capture its final scan, then archive records before stopping billing.
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Second):
				}
				if _, err = fetchBoatRecord(ctx, p, m.ID, h, name); err != nil {
					return err
				}
				return p.Stop(ctx, m.ID)
			}
		}
		if !found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// Runtime-only secrets: /run is never included in the saved template. Boat
// injects these environment variables into API commands at each VM start.
func activateBoatCapture(ctx context.Context, p EphemeralProvider, id string) error {
	script := `import os, subprocess, tempfile
keys = ["LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_ENDPOINT"]
if not os.environ.get("LAYER_API_KEY") or not os.environ.get("LAYER_NAMESPACE"):
    raise SystemExit("Boat environment requires LAYER_API_KEY and LAYER_NAMESPACE")
fd,path = tempfile.mkstemp(prefix="hev-capture-",dir="/tmp")
try:
    with os.fdopen(fd,"w") as f:
        for key in keys:
            if key in os.environ:
                value=os.environ[key]
                if "\n" in value or "\r" in value: raise SystemExit("invalid capture environment")
                f.write(key+"="+chr(34)+value.replace(chr(92),chr(92)*2).replace(chr(34),chr(92)+chr(34))+chr(34)+"\n")
    subprocess.run(["sudo","install","-m","600",path,"/run/hev-capture.env"],check=True)
finally:
    os.unlink(path)
subprocess.run(["sudo","systemctl","restart","hevd.service"],check=True)
subprocess.run(["sudo","systemctl","is-active","--quiet","hevd.service"],check=True)
`
	_, err := p.Exec(ctx, id, "python3 -c "+Quote(script))
	return err
}

func boatArchiveCommand(source, temp string) string {
	return "d=$(mktemp -d " + Quote(temp) + ") && tar --exclude=wt -czf \"$d/record.tar.gz\" -C " + Quote(source) + " . && split -b 4194304 \"$d/record.tar.gz\" \"$d/part-\" && printf '%s\\n' \"$d\"/part-*"
}

func resumeBoat(ctx context.Context, p EphemeralProvider, h Host, id string, ttl time.Duration) error {
	unlock, err := lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	machines, err := p.List(ctx, h.Name)
	if err != nil {
		return err
	}
	live := 0
	for _, m := range machines {
		if m.ID == id && m.live() {
			return nil
		}
		if m.live() {
			live++
		}
	}
	if err = CheckCap(live, h.Cap); err != nil {
		return err
	}
	return p.Resume(ctx, id, ttl)
}
