package dispatch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
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

const boatFilesLimit = 5 * 1024 * 1024
const boatCopyChunk = (boatFilesLimit/4)*3 - 3

var ephemeralProvider = func(h Host) (EphemeralProvider, error) { return NewBoat(h) }
var boatWatchLauncher = launchBoatWatch
var boatWatchPoll = 5 * time.Second
var boatWatchDrain = 10 * time.Second
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
		if h.AutoStopMinutes >= o.Timeout+10 {
			return time.Duration(h.AutoStopMinutes) * time.Minute
		}
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
func boatRPC(ctx context.Context, h Host, q Request) (r Response, err error) {
	r = Response{Version: version.Version}
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
				continue
			}
			if m.live() {
				r.Running++
			}
			rows := boatCachedRows(h.Name, name)
			if m.live() {
				if live, e := providerRPC(ctx, p, m.ID, Request{Action: "list"}); e == nil {
					rows = live
					for _, row := range live.Rows {
						if row.Name == name && !rowIsActive(row) {
							_, _ = fetchBoatRecord(ctx, p, m.ID, h, name)
							_ = boatCleanup(p, m.ID, false)
						}
					}
				}
			} else if files, e := cachedBoatFiles(name, m.ID, h.Name); e == nil {
				_ = notifyBoatRecord(ctx, h, name, files)
			}
			if len(rows.Rows) == 0 {
				state := m.State
				if !m.live() {
					state = "stopped-unfetched"
				}
				rows.Rows = []Row{{Meta: Meta{Order: Order{Name: name, Host: h.Name}}, State: state}}
			}
			r.Rows = append(r.Rows, rows.Rows...)
			r.Names = append(r.Names, name)
		}
		entries, _ := os.ReadDir(filepath.Join(Root(), ".remote", h.Name))
		for _, entry := range entries {
			if !entry.IsDir() || !ValidName(entry.Name()) {
				continue
			}
			present := false
			for _, m := range machines {
				if m.Labels["hev.order"] == entry.Name() {
					present = true
				}
			}
			if present {
				continue
			}
			if _, e := os.Stat(filepath.Join(boatRecordDir(h.Name, entry.Name()), "admission-error.txt")); e == nil {
				r.Rows = append(r.Rows, boatCachedRows(h.Name, entry.Name()).Rows...)
			}
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
		if h.AutoStopMinutes > 0 && h.AutoStopMinutes < q.Meta.Timeout+10 {
			return r, fmt.Errorf("Boat auto_stop_minutes must cover order timeout plus ten minutes")
		}
		// Serialize admission among processes on this dispatcher. The provider list
		// remains authoritative; independent dispatchers need separate host labels.
		unlock, err := lockRoot()
		if err != nil {
			return r, err
		}
		locked := true
		defer func() {
			if locked {
				unlock()
			}
		}()
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
			fresh, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if matches, e := p.List(fresh, h.Name); e == nil {
				for _, m := range matches {
					if m.Labels["hev.order"] == q.Name {
						err = errors.Join(err, boatCleanup(p, m.ID, true))
					}
				}
			}
			recordBoatAdmission(h, q, err)
			return r, err
		}
		unlock()
		locked = false
		admitted := false
		defer func() {
			if !admitted {
				cleanupErr := boatCleanup(p, machine.ID, true)
				err = errors.Join(err, cleanupErr)
				recordBoatAdmission(h, q, err)
			}
		}()
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
		dir := boatRecordDir(h.Name, q.Name)
		_ = os.MkdirAll(dir, 0700)
		_ = WriteJSON(filepath.Join(dir, "meta.json"), q.Meta)
		_ = os.Remove(filepath.Join(dir, "admission-error.txt"))
		admitted = true
		return r, nil
	}
	if machine.ID == "" {
		dir := boatRecordDir(h.Name, q.Name)
		if _, e := os.Stat(filepath.Join(dir, "admission-error.txt")); e == nil {
			if q.Action == "clean" {
				return Response{Message: "failed admission retained; no VM"}, nil
			}
			if q.Action == "show" {
				files := map[string]string{}
				for _, f := range []string{"meta.json", "result.json", "admission-error.txt"} {
					data, _ := os.ReadFile(filepath.Join(dir, f))
					files[f] = string(data)
				}
				return Response{Files: files}, nil
			}
		}
		return r, fmt.Errorf("Boat order not found")
	}
	if q.Action == "validate" {
		return r, fmt.Errorf("validation on Boat is unsupported")
	}
	if q.Action == "stop" && machine.live() {
		defer func() { err = errors.Join(err, boatCleanup(p, machine.ID, false)) }()
	}
	if q.Action == "stop" && machine.State == "archived" {
		return Response{Message: "Boat VM already stopped"}, nil
	}
	if q.Action == "clean" && machine.State == "archived" {
		if files, e := cachedBoatFiles(q.Name, machine.ID, h.Name); e == nil {
			if err = notifyBoatRecord(ctx, h, q.Name, files); err != nil {
				return r, err
			}
			if err = boatCleanup(p, machine.ID, true); err != nil {
				return r, err
			}
			return Response{Message: "deleted Boat VM; kept ~/dispatch/" + q.Name}, nil
		}
	}
	resumed := false
	successful := false
	if !machine.live() {
		if q.Action == "show" || q.Action == "logs" {
			files, e := cachedBoatFiles(q.Name, machine.ID, h.Name)
			if e != nil {
				return r, fmt.Errorf("stopped Boat VM has no cached record; use clean to fetch")
			}
			if q.Action == "logs" {
				data, _ := os.ReadFile(filepath.Join(boatRecordDir(h.Name, q.Name), "log"))
				offset := q.Offset
				if offset < 0 || offset > int64(len(data)) {
					offset = 0
				}
				end := offset + 32768
				if end > int64(len(data)) {
					end = int64(len(data))
				}
				return Response{Log: string(data[offset:end]), Offset: end, Active: end < int64(len(data))}, nil
			}
			return Response{Files: files}, notifyBoatRecord(ctx, h, q.Name, files)
		}
		if q.Action != "clean" && q.Action != "rework" && q.Action != "say" {
			return r, fmt.Errorf("action cannot resume a stopped Boat VM")
		}
		var meta Meta
		if q.Action == "rework" || q.Action == "say" {
			if e := readJSON(filepath.Join(boatRecordDir(h.Name, q.Name), "meta.json"), &meta); e != nil {
				return r, fmt.Errorf("rework requires cached order timeout: %w", e)
			}
		}
		defer func() {
			if resumed && !successful {
				err = errors.Join(err, boatCleanup(p, machine.ID, false))
			}
		}()
		resumed = true
		if err = resumeBoat(ctx, p, h, machine.ID, boatTTL(h, meta.Order)); err != nil {
			return r, err
		}
	}
	if q.Action == "rework" || (q.Action == "say" && resumed) {
		if err = activateBoatCapture(ctx, p, machine.ID); err != nil {
			return r, err
		}
	}
	if q.Action == "clean" {
		if _, err = fetchBoatRecord(ctx, p, machine.ID, h, q.Name); err != nil {
			return r, fmt.Errorf("refusing deletion before record fetch: %w", err)
		}
		if err = boatCleanup(p, machine.ID, true); err != nil {
			return r, err
		}
		return Response{Message: "deleted Boat VM; kept ~/dispatch/" + q.Name}, nil
	}
	if q.Action == "rework" || q.Action == "say" {
		// A previous terminal record cannot authorize deletion after rework.
		_ = os.Remove(filepath.Join(boatRecordDir(h.Name, q.Name), "boat.json"))
	}
	r, err = providerRPC(ctx, p, machine.ID, q)
	if err != nil {
		return r, err
	}
	switch q.Action {
	case "show":
		if r.Files["result.json"] != "" {
			_, err = fetchBoatRecord(ctx, p, machine.ID, h, q.Name)
			err = errors.Join(err, boatCleanup(p, machine.ID, false))
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
			err = boatCleanup(p, machine.ID, false)
		}
	case "rework", "say":
		if q.Action == "say" && !resumed {
			break
		}
		if err = boatWatchLauncher(h, q.Name); err != nil {
			return r, err
		}
	}
	successful = err == nil && (q.Action == "rework" || q.Action == "say")
	return r, err
}
func rowIsActive(row Row) bool {
	return row.State == "running" || row.State == "launching" || row.State == "pending" || row.State == "launch-unconfirmed"
}
func boatCachedRows(host, name string) Response {
	var m Meta
	var result Result
	dir := boatRecordDir(host, name)
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
		return Response{}
	}
	var rp *Result
	if readJSON(filepath.Join(dir, "result.json"), &result) != nil {
		return Response{}
	}
	rp = &result
	state := State(rp, false)
	if result.AdmissionFailed {
		state = "admission-failed"
	}
	return Response{Rows: []Row{{Meta: m, State: state, PR: result.PR, Trace: result.Session, Validation: result.Validation}}}
}

type boatCache struct{ ID, Host string }

func cachedBoatFiles(name, id, host string) (map[string]string, error) {
	dir := boatRecordDir(host, name)
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
	unlock, err := lockBoat(h.Name, name, "fetch")
	if err != nil {
		return Response{}, err
	}
	defer unlock()
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
	dir := boatRecordDir(h.Name, name)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return show, err
	}
	temp, err := os.MkdirTemp(filepath.Dir(dir), ".fetch-")
	if err != nil {
		return show, err
	}
	defer os.RemoveAll(temp)
	target := dir
	dir = temp
	if err = extractBoatRecord(dir, archive); err != nil {
		return show, err
	}
	if err = os.WriteFile(filepath.Join(dir, "boat-record.tar.gz"), archive, 0600); err != nil {
		return show, err
	}
	if data, e := os.ReadFile(filepath.Join(target, "watch-error.txt")); e == nil {
		_ = os.WriteFile(filepath.Join(dir, "watch-error.txt"), data, 0600)
	}
	if err = WriteJSON(filepath.Join(dir, "boat.json"), boatCache{ID: id, Host: h.Name}); err != nil {
		return show, err
	}
	old := target + ".old"
	_ = os.RemoveAll(old)
	if _, e := os.Stat(target); e == nil {
		if err = os.Rename(target, old); err != nil {
			return show, err
		}
	}
	if err = os.Rename(dir, target); err != nil {
		_ = os.Rename(old, target)
		return show, err
	}
	_ = os.RemoveAll(old)
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
	marker := boatRecordDir(h.Name, name) + ".notified"
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
	unlock, e := lockBoat(h.Name, name, "watch")
	if e != nil {
		return e
	}
	defer unlock()
	lastError := func(err error) {
		dir := boatRecordDir(h.Name, name)
		_ = os.MkdirAll(dir, 0700)
		_ = os.WriteFile(filepath.Join(dir, "watch-error.txt"), []byte(err.Error()), 0600)
	}
	delay := boatWatchPoll
	pause := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
			if delay < time.Minute {
				delay *= 2
			}
			return true
		}
	}

	p, err := ephemeralProvider(h)
	if err != nil {
		return err
	}
	for {
		machines, err := p.List(ctx, h.Name)
		if err != nil {
			lastError(err)
			if !pause() {
				return ctx.Err()
			}
			continue
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
				lastError(err)
				if !pause() {
					return ctx.Err()
				}
				continue
			}
			delay = boatWatchPoll
			if r.Files["result.json"] != "" {
				defer func() { _ = boatCleanup(p, m.ID, false) }()
				// Give capture its final scan, then archive records before stopping billing.
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(boatWatchDrain):
				}
				_, err = fetchBoatRecord(ctx, p, m.ID, h, name)
				stopErr := boatCleanup(p, m.ID, false)
				if err != nil {
					lastError(err)
					var meta Meta
					_ = readJSON(filepath.Join(boatRecordDir(h.Name, name), "meta.json"), &meta)
					fresh, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					_ = Notify(fresh, h.Notify, meta, Result{Error: err.Error(), End: time.Now(), ExitCode: 1})
				}
				return errors.Join(err, stopErr)
			}
		}
		if !found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(boatWatchPoll):
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
	return "d=$(mktemp -d " + Quote(temp) + ") && tar --exclude=wt -czf \"$d/record.tar.gz\" -C " + Quote(source) + " . && split -b " + fmt.Sprint(boatCopyChunk) + " \"$d/record.tar.gz\" \"$d/part-\" && printf '%s\\n' \"$d\"/part-*"
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
	if err := p.Resume(ctx, id, ttl); err != nil {
		return errors.Join(err, boatCleanup(p, id, false))
	}
	return nil
}

func boatRecordDir(host, name string) string { return filepath.Join(Root(), ".remote", host, name) }
func boatCleanup(p EphemeralProvider, id string, deleteVM bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if deleteVM {
		return p.Delete(ctx, id)
	}
	return p.Stop(ctx, id)
}
func lockBoat(host, name, kind string) (func(), error) {
	dir := boatRecordDir(host, name) + "." + kind + "-lock"
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("Boat %s locked: %w", kind, err)
	}
	return func() { _ = os.Remove(dir) }, nil
}

func recordBoatAdmission(h Host, q Request, err error) {
	dir := boatRecordDir(h.Name, q.Name)
	_ = os.MkdirAll(dir, 0700)
	q.Meta.Name = q.Name
	q.Meta.Host = h.Name
	q.Meta.Ephemeral = true
	_ = WriteJSON(filepath.Join(dir, "meta.json"), q.Meta)
	_ = WriteJSON(filepath.Join(dir, "result.json"), Result{AdmissionFailed: true, ExitCode: 1, End: time.Now(), Error: fmt.Sprint(err)})
	_ = os.WriteFile(filepath.Join(dir, "admission-error.txt"), []byte(fmt.Sprint(err)), 0600)
}
