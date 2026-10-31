package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/version"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var validationNotificationLauncher = launchValidationNotification

type Request struct {
	FromValidation   bool             `json:"from_validation,omitempty"`
	ValidationRecord ValidationRecord `json:"validation_record,omitempty"`
	Offset           int64            `json:"offset,omitempty"`
	Action           string           `json:"action"`
	Name             string           `json:"name"`
	Order            string           `json:"order,omitempty"`
	Meta             Meta             `json:"meta"`
	Note             string           `json:"note,omitempty"`
	Cap              int              `json:"cap"`
	Workspace        string           `json:"workspace"`
}
type Row struct {
	Validation string `json:"validation,omitempty"`
	Meta
	State string `json:"state"`
	PR    string `json:"pr,omitempty"`
	Trace string `json:"trace,omitempty"`
}
type Response struct {
	Offset          int64             `json:"offset,omitempty"`
	Active          bool              `json:"active,omitempty"`
	Cap             int               `json:"cap"`
	Free            int               `json:"free"`
	Reachable       bool              `json:"reachable"`
	VersionMismatch bool              `json:"version_mismatch"`
	Names           []string          `json:"names,omitempty"`
	Rows            []Row             `json:"rows,omitempty"`
	Running         int               `json:"running"`
	Files           map[string]string `json:"files,omitempty"`
	Log             string            `json:"log,omitempty"`
	Message         string            `json:"message,omitempty"`
	Version         string            `json:"version,omitempty"`
	Error           string            `json:"error,omitempty"`
}

func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func RPC(ctx context.Context, h Host, q Request) (Response, error) {
	if h.Local {
		return Handle(q)
	}
	var r Response
	b, e := json.Marshal(q)
	if e != nil {
		return r, e
	}
	remote := RemoteCommand(h, "hev dispatch rpc")
	c := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=yes", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=1", h.SSH, remote)
	var errout strings.Builder
	c.Stderr = &errout
	c.Stdin = bytes.NewReader(b)
	out, e := c.Output()
	if e != nil {
		return r, fmt.Errorf("ssh: %w: %s; for mutations the outcome is unknown: inspect the named run before retrying", e, strings.TrimSpace(errout.String()))
	}
	if e = json.Unmarshal(out, &r); e != nil {
		return r, fmt.Errorf("invalid remote response: %w", e)
	}
	if r.Error != "" {
		return r, fmt.Errorf("%s", r.Error)
	}
	return r, nil
}
func Inventory(c Config, host string) map[string]Response {
	out := map[string]Response{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for n, h := range c.Hosts {
		if host != "" && host != n {
			continue
		}
		wg.Add(1)
		go func(n string, h Host) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			r, e := RPC(ctx, h, Request{Action: "list"})
			if e != nil {
				r.Error = e.Error()
			}
			r.Cap = h.Cap
			r.Free = max(0, h.Cap-r.Running)
			r.Reachable = e == nil
			r.VersionMismatch = e == nil && r.Version != version.Version
			mu.Lock()
			out[n] = r
			mu.Unlock()
		}(n, h)
	}
	wg.Wait()
	return out
}
func Handle(q Request) (r Response, handleErr error) {
	r = Response{Version: version.Version}
	if e := os.MkdirAll(Root(), 0700); e != nil {
		return r, e
	}
	live, e := liveNames()
	if e != nil {
		return r, e
	}
	if e = addReservations(live); e != nil {
		return r, e
	}
	r.Running = len(live)
	if q.Action == "list" {
		dirs, e := os.ReadDir(Root())
		if e != nil {
			return r, e
		}
		for _, d := range dirs {
			if d.IsDir() && ValidName(d.Name()) {
				r.Names = append(r.Names, d.Name())
			}
			var m Meta
			if !d.IsDir() || readJSON(filepath.Join(Root(), d.Name(), "meta.json"), &m) != nil {
				continue
			}
			var result Result
			var rp *Result
			if readJSON(filepath.Join(Root(), d.Name(), "result.json"), &result) == nil {
				reconcileLaunch(filepath.Join(Root(), d.Name()), &result)
				rp = &result
			}
			m.SameHost = m.SameHost || result.SameHost
			r.Rows = append(r.Rows, Row{Meta: m, State: State(rp, live[d.Name()]), PR: result.PR, Trace: result.Session, Validation: result.Validation})
		}
		return r, nil
	}
	if !ValidName(q.Name) {
		return r, fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), q.Name)
	var m Meta
	if q.Action == "say" {
		if strings.TrimSpace(q.Note) == "" {
			return r, fmt.Errorf("message required")
		}
		if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
			return r, fmt.Errorf("order not found: %s", q.Name)
		}
		unlock, err := lockRoot()
		if err != nil {
			return r, err
		}
		live, err = liveNames()
		if err == nil {
			err = addReservations(live)
		}
		if err != nil {
			unlock()
			return r, err
		}
		if live[q.Name] {
			defer unlock()
			f, err := os.OpenFile(filepath.Join(dir, "inbox.md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return r, err
			}
			_, err = f.WriteString("\n## " + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + q.Note + "\n")
			closeErr := f.Close()
			if err != nil {
				return r, err
			}
			r.Message = "sent " + q.Name
			return r, closeErr
		}
		unlock()
		q.Action = "rework"
	}
	if q.Action == "start" || q.Action == "rework" {
		unlock, e := lockRoot()
		if e != nil {
			return r, e
		}
		locked := true
		defer func() {
			if locked {
				unlock()
			}
		}()
		live, e = liveNames()
		if e != nil {
			return r, e
		}
		if e = addReservations(live); e != nil {
			return r, e
		}
		if q.Action == "rework" && live[q.Name] {
			return r, fmt.Errorf("stop running order before rework")
		}
		if e = CheckCap(len(live), q.Cap); e != nil {
			return r, e
		}
		if q.Action == "start" {
			var o Order
			o, e = Parse(q.Order)
			if e != nil {
				return r, e
			}
			if (o.Name != q.Name && q.Meta.Parent != o.Name) || q.Meta.Name != q.Name {
				return r, fmt.Errorf("order name mismatch")
			}
			m = q.Meta
			if m.Parent != "" {
				cfg, err := loadTarget(m.ConfigPath, m.Host)
				if err != nil {
					return r, fmt.Errorf("validator config: %w", err)
				}
				route, ok := cfg.Hosts[m.ParentHostName]
				if !ok {
					err := fmt.Errorf("validator has no parent route for host %q; add a matching hosts entry to the validator config", m.ParentHostName)
					if e = os.Mkdir(dir, 0700); e != nil {
						return r, e
					}
					if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
						return r, e
					}
					if e = WriteJSON(filepath.Join(dir, "result.json"), Result{AdmissionFailed: true, SameHost: m.SameHost, ExitCode: 1, End: time.Now().UTC(), Validation: "fail", Error: err.Error()}); e != nil {
						return r, e
					}
					return r, err
				}
				m.ParentHost = route
				m.ParentHost.Env = nil
			}
			if m.Parent == "" {
				m.Order = o
			}
			m.Host = q.Meta.Host
			if m.Parent == "" {
				m.Branch = "d/" + q.Name
			}
			m.Start = time.Now().UTC()
			if e = os.Mkdir(dir, 0700); e != nil {
				return r, fmt.Errorf("name already exists or cannot create: %w", e)
			}
			defer func() {
				if handleErr != nil {
					if errors.Is(handleErr, errLaunchUnconfirmed) {
						_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
						recordUnconfirmed(dir, m, handleErr)
						return
					}
					// Keep even early setup failures visible and explicitly removable.
					_ = WriteJSON(filepath.Join(dir, "meta.json"), m)
					_ = WriteJSON(filepath.Join(dir, "result.json"), Result{AdmissionFailed: true, ExitCode: 1, Start: m.Start, End: time.Now().UTC(), Branch: m.Branch, Error: handleErr.Error()})
				}
			}()
			if e = os.WriteFile(filepath.Join(dir, "order.md"), []byte(q.Order), 0600); e != nil {
				return r, e
			}
			if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
				return r, e
			}
			if e = reserve(dir); e != nil {
				return r, e
			}
			defer os.Remove(filepath.Join(dir, "admission.json"))
			unlock()
			locked = false
			cfg, err := loadTarget(m.ConfigPath, m.Host)
			if err != nil && (!os.IsNotExist(err) || m.ConfigPath != "") {
				return r, fmt.Errorf("target config: %w", err)
			}
			resolveLaunch(&m, cfg)
			workspace := q.Workspace
			if workspace == "" {
				workspace = cfg.Workspace
			}
			repo := resolveRepo(m.Repo, workspace)
			m.ResolvedRepo = repo
			if _, e = command("", "git", "-C", repo, "fetch", "origin"); e != nil {
				return r, e
			}
			gitArgs := []string{"-C", repo, "worktree", "add", "-b", m.Branch, filepath.Join(dir, "wt"), "origin/" + m.Base}
			if m.Parent != "" {
				remoteHead, err := command(repo, "git", "rev-parse", "origin/"+m.Branch)
				if err != nil || remoteHead != m.Revision {
					e = fmt.Errorf("recorded head is not the pushed branch head")
					return r, e
				}
				gitArgs = []string{"-C", repo, "worktree", "add", "--detach", filepath.Join(dir, "wt"), m.Revision}
				_ = os.MkdirAll(filepath.Join(dir, "evidence"), 0700)
			}
			if _, e = command("", append([]string{"git"}, gitArgs...)...); e != nil {
				return r, e
			}
			m.WorktreeCreated = true
			m.BranchCreated = m.Parent == ""
		} else {
			if e = readJSON(filepath.Join(dir, "meta.json"), &m); e != nil {
				return r, e
			}
			if _, e = os.Stat(filepath.Join(dir, "wt")); e != nil {
				return r, fmt.Errorf("worktree was cleaned")
			}
			if q.FromValidation {
				q.Note, e = validationNote(dir)
				if e != nil {
					return r, e
				}
			}
			if strings.TrimSpace(q.Note) == "" {
				return r, fmt.Errorf("rework note required")
			}
			f, e := os.OpenFile(filepath.Join(dir, "order.md"), os.O_APPEND|os.O_WRONLY, 0600)
			if e != nil {
				return r, e
			}
			_, e = f.WriteString("\n## Rework\n\n" + q.Note + "\n")
			_ = f.Close()
			if e != nil {
				return r, e
			}
			for _, file := range []string{"result.json", "report.md", "status.md", "log", "inbox.md"} {
				p := filepath.Join(dir, file)
				if _, e = os.Stat(p); e == nil {
					if e = os.Rename(p, p+"."+fmt.Sprint(time.Now().UnixNano())); e != nil {
						return r, e
					}
				}
			}
			cfg, err := loadTarget(m.ConfigPath, m.Host)
			if err != nil && (!os.IsNotExist(err) || m.ConfigPath != "") {
				return r, err
			}
			resolveLaunch(&m, cfg)
			m.Start = time.Now().UTC()
			if e = reserve(dir); e != nil {
				return r, e
			}
			defer os.Remove(filepath.Join(dir, "admission.json"))
			if !q.FromValidation {
				if e = resetValidation(dir); e != nil {
					return r, e
				}
			}
			unlock()
			locked = false
		}
		_ = os.Remove(filepath.Join(dir, "stop"))
		_ = os.Remove(filepath.Join(dir, "supervisor.json"))
		if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
			return r, e
		}
		if e = launch(dir, q.Name); e != nil {
			if q.Action == "rework" && errors.Is(e, errLaunchUnconfirmed) {
				recordUnconfirmed(dir, m, e)
			}
			return r, e
		}
		r.Message = "started " + q.Name
		return r, nil
	}
	if e = readJSON(filepath.Join(dir, "meta.json"), &m); e != nil {
		return r, fmt.Errorf("order not found: %s", q.Name)
	}
	switch q.Action {
	case "validation-reserve":
		unlock, err := lockRoot()
		if err != nil {
			return r, err
		}
		defer unlock()
		r.Message, e = reserveValidation(dir, q.Name)
	case "validation-result":
		unlock, err := lockRoot()
		if err != nil {
			return r, err
		}
		e = recordValidation(dir, q.ValidationRecord)
		unlock()
		if e == nil && q.ValidationRecord.Verdict != nil {
			for _, record := range validationRecords(dir) {
				if record.Name == q.ValidationRecord.Name && record.State != "stale" {
					validationNotificationLauncher(dir, m)
					break
				}
			}
		}
	case "show":
		var pending Result
		if readJSON(filepath.Join(dir, "result.json"), &pending) == nil {
			reconcileLaunch(dir, &pending)
		}
		r.Files = map[string]string{}
		for _, f := range []string{"meta.json", "order.md", "status.md", "report.md", "result.json", "inbox.md", "validation.json", "verdict.json"} {
			b, _ := os.ReadFile(filepath.Join(dir, f))
			if f == "status.md" && len(b) > 12000 {
				b = b[len(b)-12000:]
			}
			if f == "meta.json" {
				var public Meta
				if json.Unmarshal(b, &public) == nil {
					b, _ = json.MarshalIndent(public, "", "  ")
				}
			}
			r.Files[f] = string(b)
		}
		var result Result
		_ = readJSON(filepath.Join(dir, "result.json"), &result)
		if result.Session != "" {
			r.Message = "hev trace " + result.Session
		}
	case "logs":
		f, err := os.Open(filepath.Join(dir, "log"))
		if err != nil && !os.IsNotExist(err) {
			return r, err
		}
		if err == nil {
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				return r, err
			}
			offset := q.Offset
			if offset < 0 || offset > info.Size() {
				offset = 0
			}
			if _, err = f.Seek(offset, io.SeekStart); err != nil {
				return r, err
			}
			b, err := io.ReadAll(f)
			if err != nil {
				return r, err
			}
			r.Log, r.Offset = string(b), offset+int64(len(b))
			r.Active = live[q.Name] || r.Offset < info.Size()
		} else {
			r.Active = live[q.Name]
		}
	case "stop":
		e = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
		if !supervisorLive(dir) {
			e = stopHarness(dir)
			if e == nil {
				var result Result
				_ = readJSON(filepath.Join(dir, "result.json"), &result)
				if result.End.IsZero() {
					result.ExitCode, result.Stopped, result.Signal = 143, true, "killed"
					result.End, result.Branch = time.Now().UTC(), m.Branch
					if result.Start.IsZero() {
						result.Start = m.Start
					}
					result.Head, _ = command(filepath.Join(dir, "wt"), "git", "rev-parse", "HEAD")
					e = WriteJSON(filepath.Join(dir, "result.json"), result)
				}
			}
		}
		r.Message = "stopping " + q.Name
	case "clean":
		if live[q.Name] {
			return r, fmt.Errorf("stop running order before clean")
		}
		var result Result
		if readJSON(filepath.Join(dir, "result.json"), &result) == nil {
			reconcileLaunch(dir, &result)
		}
		if result.LaunchUnconfirmed {
			return r, fmt.Errorf("launch unconfirmed; inspect ls/show after launch grace period before clean")
		}
		failedAdmission := readJSON(filepath.Join(dir, "result.json"), &result) == nil && result.AdmissionFailed
		if _, err := os.Stat(filepath.Join(dir, "wt")); err == nil && (!failedAdmission || m.WorktreeCreated) {
			_, e = command("", "git", "-C", filepath.Join(dir, "wt"), "worktree", "remove", filepath.Join(dir, "wt"))
		}
		if e == nil && failedAdmission {
			// A failed supervisor launch may already have created the branch.
			// Only remove a merged implementation branch, never the
			// existing source branch used by a detached validator worktree.
			if m.BranchCreated && m.Parent == "" && m.Branch != "" {
				if _, err := command("", "git", "-C", m.ResolvedRepo, "show-ref", "--verify", "refs/heads/"+m.Branch); err == nil {
					_, e = command("", "git", "-C", m.ResolvedRepo, "branch", "-d", "--", m.Branch)
				}
			}
			if e == nil {
				e = os.RemoveAll(dir)
			}
		}
		r.Message = "cleaned " + q.Name
	default:
		return r, fmt.Errorf("unknown action")
	}
	return r, e
}
func SortedHosts(m map[string]Response) []string {
	a := []string{}
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}

// RemoteCommand applies the configured Unix executable search path.
func RemoteCommand(h Host, command string) string {
	remote := command
	if h.OS != "windows" && len(h.PathPrefix) > 0 {
		parts := []string{}
		for _, p := range h.PathPrefix {
			if strings.HasPrefix(p, "~/") {
				parts = append(parts, "\"$HOME\"/"+Quote(p[2:]))
			} else {
				parts = append(parts, Quote(p))
			}
		}
		remote = "PATH=" + strings.Join(parts, ":") + ":\"$PATH\" " + remote
	}
	return remote
}

func resolveRepo(repo, workspace string) string {
	repo = Expand(repo)
	// Recognize Windows drive and UNC paths even when inspecting from Unix.
	if absoluteRepo(repo) {
		return repo
	}
	if workspace == "" {
		workspace = "~/workspace"
	}
	return filepath.Join(Expand(workspace), repo)
}

func absoluteRepo(repo string) bool {
	return strings.HasPrefix(repo, "/") || filepath.IsAbs(Expand(repo)) || trace.WindowsPath(repo)
}
