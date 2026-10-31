package dispatch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/version"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Request struct {
	Action    string `json:"action"`
	Name      string `json:"name"`
	Order     string `json:"order,omitempty"`
	Meta      Meta   `json:"meta"`
	Note      string `json:"note,omitempty"`
	Cap       int    `json:"cap"`
	Workspace string `json:"workspace"`
}
type Row struct {
	Meta
	State string `json:"state"`
	PR    string `json:"pr,omitempty"`
	Trace string `json:"trace,omitempty"`
}
type Response struct {
	Cap             int               `json:"cap"`
	Free            int               `json:"free"`
	Reachable       bool              `json:"reachable"`
	VersionMismatch bool              `json:"version_mismatch"`
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
	payload := base64.StdEncoding.EncodeToString(b)
	remote := "hev dispatch rpc " + Quote(payload)
	if h.OS == "windows" {
		remote = "hev dispatch rpc " + payload
	} else if len(h.PathPrefix) > 0 {
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
	c := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=yes", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=1", h.SSH, remote)
	var errout strings.Builder
	c.Stderr = &errout
	out, e := c.Output()
	if e != nil {
		return r, fmt.Errorf("ssh: %w: %s", e, strings.TrimSpace(errout.String()))
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
func Handle(q Request) (Response, error) {
	r := Response{Version: version.Version}
	if e := os.MkdirAll(Root(), 0700); e != nil {
		return r, e
	}
	live, e := liveNames()
	if e != nil {
		return r, e
	}
	r.Running = len(live)
	if q.Action == "list" {
		dirs, e := os.ReadDir(Root())
		if e != nil {
			return r, e
		}
		for _, d := range dirs {
			var m Meta
			if !d.IsDir() || readJSON(filepath.Join(Root(), d.Name(), "meta.json"), &m) != nil {
				continue
			}
			var result Result
			var rp *Result
			if readJSON(filepath.Join(Root(), d.Name(), "result.json"), &result) == nil {
				rp = &result
			}
			r.Rows = append(r.Rows, Row{m, State(rp, live[d.Name()]), result.PR, result.Session})
		}
		return r, nil
	}
	if !ValidName(q.Name) {
		return r, fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), q.Name)
	var m Meta
	if q.Action == "start" || q.Action == "rework" {
		unlock, e := lockRoot()
		if e != nil {
			return r, e
		}
		defer unlock()
		live, e = liveNames()
		if e != nil {
			return r, e
		}
		if q.Action == "rework" && live[q.Name] {
			return r, fmt.Errorf("stop running order before rework")
		}
		if e = CheckCap(len(live), q.Cap); e != nil {
			return r, e
		}
		if q.Action == "start" {
			o, e := Parse(q.Order)
			if e != nil {
				return r, e
			}
			if o.Name != q.Name || q.Meta.Name != q.Name {
				return r, fmt.Errorf("order name mismatch")
			}
			m = q.Meta
			m.Order = o
			m.Host = q.Meta.Host
			m.Branch = "d/" + q.Name
			m.Start = time.Now().UTC()
			if e = os.Mkdir(dir, 0700); e != nil {
				return r, fmt.Errorf("name already exists or cannot create: %w", e)
			}
			if e = os.WriteFile(filepath.Join(dir, "order.md"), []byte(q.Order), 0600); e != nil {
				return r, e
			}
			if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
				return r, e
			}
			repo := Expand(m.Repo)
			if !filepath.IsAbs(repo) {
				workspace := q.Workspace
				if workspace == "" {
					workspace = "~/workspace"
				}
				repo = filepath.Join(Expand(workspace), repo)
			}
			if _, e = command("", "git", "-C", repo, "fetch", "origin"); e != nil {
				return r, e
			}
			if _, e = command("", "git", "-C", repo, "worktree", "add", "-b", m.Branch, filepath.Join(dir, "wt"), "origin/"+m.Base); e != nil {
				return r, e
			}
		} else {
			if e = readJSON(filepath.Join(dir, "meta.json"), &m); e != nil {
				return r, e
			}
			if _, e = os.Stat(filepath.Join(dir, "wt")); e != nil {
				return r, fmt.Errorf("worktree was cleaned")
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
			for _, file := range []string{"result.json", "report.md", "status.md", "log"} {
				p := filepath.Join(dir, file)
				if _, e = os.Stat(p); e == nil {
					if e = os.Rename(p, p+"."+fmt.Sprint(time.Now().UnixNano())); e != nil {
						return r, e
					}
				}
			}
			m.Start = time.Now().UTC()
		}
		_ = os.Remove(filepath.Join(dir, "stop"))
		_ = os.Remove(filepath.Join(dir, "supervisor.json"))
		if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
			return r, e
		}
		if e = launch(dir, q.Name); e != nil {
			return r, e
		}
		r.Message = "started " + q.Name
		return r, nil
	}
	if e = readJSON(filepath.Join(dir, "meta.json"), &m); e != nil {
		return r, fmt.Errorf("order not found: %s", q.Name)
	}
	switch q.Action {
	case "show":
		r.Files = map[string]string{}
		for _, f := range []string{"meta.json", "status.md", "report.md", "result.json"} {
			b, _ := os.ReadFile(filepath.Join(dir, f))
			if f == "status.md" && len(b) > 12000 {
				b = b[len(b)-12000:]
			}
			r.Files[f] = string(b)
		}
		var result Result
		_ = readJSON(filepath.Join(dir, "result.json"), &result)
		if result.Session != "" {
			r.Message = "hev trace " + result.Session
		}
	case "logs":
		b, e := os.ReadFile(filepath.Join(dir, "log"))
		if e != nil && !os.IsNotExist(e) {
			return r, e
		}
		r.Log = string(b)
	case "stop":
		if live[q.Name] {
			e = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
		}
		r.Message = "stopping " + q.Name
	case "clean":
		if live[q.Name] {
			return r, fmt.Errorf("stop running order before clean")
		}
		if _, err := os.Stat(filepath.Join(dir, "wt")); err == nil {
			_, e = command("", "git", "-C", filepath.Join(dir, "wt"), "worktree", "remove", filepath.Join(dir, "wt"))
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
