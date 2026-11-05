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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var validationNotificationLauncher = launchValidationNotification

var runLauncher = launch

type Request struct {
	excludeFinished string
	Dequeue         bool `json:"dequeue,omitempty"`
	AdmissionOnly   bool `json:"admission_only,omitempty"`
	// ReserveClass marks a source start's probe of its target: it also
	// admits and reserves a classed order's slot there (see reserveClassSlot).
	ReserveClass     bool             `json:"reserve_class,omitempty"`
	Orchestrator     string           `json:"orchestrator,omitempty"`
	Transfer         []byte           `json:"transfer,omitempty"`
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
	// IDs names the notices an orchestrator-notice-ack acknowledges.
	IDs []string `json:"ids,omitempty"`
}
type Row struct {
	Position     int      `json:"position,omitempty"`
	Kind         string   `json:"kind"`
	Orchestrator string   `json:"orchestrator"`
	Head         string   `json:"head_sha,omitempty"`
	Schedule     Schedule `json:"schedule"`
	Error        string   `json:"error,omitempty"`
	Validation   string   `json:"validation,omitempty"`
	Meta
	State string `json:"state"`
	PR    string `json:"pr,omitempty"`
	Trace string `json:"trace,omitempty"`
}
type Response struct {
	AttachHost      *Host             `json:"attach_host,omitempty"`
	ImportRefused   bool              `json:"import_refused,omitempty"`
	History         *History          `json:"history,omitempty"`
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
	DeliveryStatus  string            `json:"delivery_status,omitempty"`
	Message         string            `json:"message,omitempty"`
	Version         string            `json:"version,omitempty"`
	Error           string            `json:"error,omitempty"`
	// ClassReserved confirms a ReserveClass probe. A target too old to
	// reserve leaves it false, and the owner refuses rather than bypass.
	ClassReserved bool `json:"class_reserved,omitempty"`
	// Notices are pending orchestrator notices, oldest first.
	Notices []OrchestratorNotice `json:"notices,omitempty"`
}

func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func RPC(ctx context.Context, h Host, q Request) (Response, error) {
	requestOrchestratorIdentity(&q)
	if q.Orchestrator != "" && !q.AdmissionOnly && !workerInternal(q.Action) {
		// Pin a live say to inbox-only delivery so a liveness race cannot turn
		// a non-launching message into unreserved rework on the target.
		if q.Action == "say" {
			r, e := rpcTransport(ctx, h, Request{Action: "show", Name: q.Name, Orchestrator: q.Orchestrator})
			if e != nil {
				return r, e
			}
			if r.Active {
				q.Action = "peer-send"
			}
		}
		release, e := reserveOrchestratorOwner(q, h)
		if e != nil {
			return Response{}, e
		}
		defer release()
	}
	return rpcTransport(ctx, h, q)
}

// Forwarding uses the transport directly: the origin has already reserved its
// owner budget and a forwarded mutation must not reacquire that same lock.
func rpcTransport(ctx context.Context, h Host, q Request) (Response, error) {
	requestOrchestratorIdentity(&q)
	if q.Action == "start" && q.Meta.Sender == "" {
		q.Meta.Sender = LocalSender()
	}
	if h.Local {
		return Handle(q)
	}
	var r Response
	b, e := json.Marshal(q)
	if e != nil {
		return r, e
	}
	remote := RemoteCommand(h, "hev dispatch rpc")
	c := helperCommand(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=yes", "-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=1", h.SSH, remote)
	var errout strings.Builder
	c.Stderr = &errout
	c.Stdin = bytes.NewReader(b)
	out, e := c.Output()
	if e != nil {
		return r, transportError{fmt.Errorf("ssh: %w: %s; for mutations the outcome is unknown: inspect the named run before retrying", e, strings.TrimSpace(errout.String()))}
	}
	if e = json.Unmarshal(out, &r); e != nil {
		return r, transportError{fmt.Errorf("invalid remote response: %w", e)}
	}
	if r.Error != "" {
		return r, fmt.Errorf("%s", r.Error)
	}
	return r, nil
}

// transportError marks an RPC whose remote outcome is unknown. A remote that
// answered with an error is definitive and is returned unwrapped.
type transportError struct{ err error }

func (e transportError) Error() string { return e.err.Error() }
func (e transportError) Unwrap() error { return e.err }

var errRunExists = errors.New("name already exists")

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
			if e == nil && r.Cap <= 0 {
				r.Cap = h.Cap
				r.Free = max(0, r.Cap-r.Running)
			}
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
func workerInternal(action string) bool {
	return action == "validation-reserve" || action == "validation-result"
}

func Handle(q Request) (r Response, handleErr error) {
	r = Response{Version: version.Version}
	requestOrchestratorIdentity(&q)
	if workerInternal(q.Action) {
		q.Orchestrator = ""
	}
	if e := admitOrchestrator(q); e != nil {
		return r, e
	}
	if q.AdmissionOnly {
		if q.Action == "start" && ValidName(q.Name) && exists(filepath.Join(Root(), q.Name)) {
			return r, fmt.Errorf("%w: %s", errRunExists, q.Name)
		}
		if q.Action == "start" && q.ReserveClass {
			return reserveClassSlot(q)
		}
		return r, nil
	}
	// The pull route: list pending owner notices, then acknowledge the ones
	// delivered into the caller's inbox.
	switch q.Action {
	case "orchestrator-notices":
		n, e := PendingOrchestratorNotices(q.Name)
		r.Notices = n
		return r, e
	case "orchestrator-notice-ack":
		return r, AckOrchestratorNotices(q.IDs)
	}
	if q.Action == "orchestrator-apply" {
		s, e := ApplyOrchestrator(q.Order, false)
		if e != nil {
			return r, e
		}
		if e = os.WriteFile(filepath.Join(OrchestratorDir(s.Name), "order.md"), []byte(q.Note), 0600); e != nil {
			return r, e
		}
		return r, LaunchOrchestrator(s.Name)
	}
	if strings.HasPrefix(q.Name, "o-") && (q.Action == "say" || q.Action == "peer-send" || q.Action == "stop" || q.Action == "show") {
		name := strings.TrimPrefix(q.Name, "o-")
		if _, e := LoadOrchestrator(name); e != nil {
			return r, e
		}
		switch q.Action {
		case "say", "peer-send":
			e := SayOrchestrator(name, q.Note)
			if e == nil {
				e = LaunchOrchestrator(name)
			}
			return r, e
		case "stop":
			return r, OrchestratorControl(name, "stop", "")
		case "show":
			r.Files = map[string]string{}
			for _, f := range []string{"order.md", "spec.md", "checkpoint.md", "state.json"} {
				b, e := os.ReadFile(filepath.Join(OrchestratorDir(name), f))
				if e != nil && !os.IsNotExist(e) {
					return r, e
				}
				r.Files[f] = string(b)
			}
			return r, nil
		}
	}

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
	if q.Action == "sweep" {
		return r, SweepQueue()
	}
	if q.Action == "list" {
		policy, err := targetPolicy()
		if err != nil {
			return r, fmt.Errorf("target admission config: %w", err)
		}
		r.Cap, r.Free = policy.Cap, max(0, policy.Cap-r.Running)
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
			if m.Imported {
				continue
			}
			if m.Forward != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				remote, err := routeRPC(ctx, *m.Forward, Request{Action: "show", Name: m.Name})
				cancel()
				if err == nil {
					m = Meta{}
					result = Result{}
					rp = &result
					_ = json.Unmarshal([]byte(remote.Files["meta.json"]), &m)
					_ = json.Unmarshal([]byte(remote.Files["result.json"]), &result)
					live[d.Name()] = remote.Active
				} else {
					result.Error = err.Error()
				}
			}
			m.SameHost = m.SameHost || result.SameHost
			r.Rows = append(r.Rows, Row{Kind: "run", Schedule: loadSchedule(filepath.Join(Root(), d.Name())), Meta: m, State: State(rp, live[d.Name()]), PR: result.PR, Head: result.Head, Trace: result.Session, Validation: result.Validation, Error: result.Error})
		}
		unlockQueue, err := lockRoot()
		if err != nil {
			return r, err
		}
		entries, err := queueEntries()
		unlockQueue()
		if err != nil {
			return r, err
		}
		for i, entry := range entries {
			r.Names = append(r.Names, entry.Request.Name)
			r.Rows = append(r.Rows, Row{Kind: "run", Meta: entry.Request.Meta, State: "queued", Position: i + 1})
		}
		if e := addOrchestratorRows(&r); e != nil {
			return r, e
		}
		return r, nil
	}
	if !ValidName(q.Name) {
		return r, fmt.Errorf("invalid name")
	}
	dir := filepath.Join(Root(), q.Name)
	var m Meta
	if q.Action == "attach-route" {
		if err := readJSON(filepath.Join(dir, "meta.json"), &m); err != nil {
			return r, err
		}
		r.AttachHost = m.Forward
		return r, nil
	}
	if readJSON(filepath.Join(dir, "meta.json"), &m) == nil && m.Forward != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return routeRPC(ctx, *m.Forward, q)
	}
	if q.Action == "show" || q.Action == "stop" {
		if response, found, err := controlQueued(q); found || err != nil {
			return response, err
		}
	}
	originalAction := q.Action
	if q.Action == "retry" {
		q.Action = "rework"
		if strings.TrimSpace(q.Note) == "" {
			q.Note = "Retry the existing order."
		}
	}
	if q.Action == "amend" {
		return amend(q)
	}
	if q.Action == "amend-check" || q.Action == "amend-import" {
		return receiveAmend(q, live)
	}
	if q.Action == "say" || q.Action == "peer-send" {
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
		// A message may have waited behind a host move's root lock.
		// Resolve the owner again before choosing the inbox to append to.
		if readJSON(filepath.Join(dir, "meta.json"), &m) == nil && m.Forward != nil {
			unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			return routeRPC(ctx, *m.Forward, q)
		}
		live, err = liveNames()
		if err == nil {
			err = addReservations(live)
		}
		if err != nil {
			unlock()
			return r, err
		}
		if live[q.Name] || q.Action == "peer-send" {
			defer unlock()
			f, err := os.OpenFile(filepath.Join(dir, "inbox.md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return r, err
			}
			_, err = f.WriteString("\n## " + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + q.Note + "\n")
			syncErr := f.Sync()
			closeErr := f.Close()
			if err == nil {
				err = syncErr
			}
			if err != nil {
				return r, err
			}
			r.Message = "sent " + q.Name
			if q.Action == "peer-send" {
				r.DeliveryStatus = "delivered to run inbox"
				r.Message = "delivered to run inbox"
				if !live[q.Name] {
					r.DeliveryStatus = "held for ended run"
					r.Message = "held in ended run inbox; no rework started"
				}
			}
			if live[q.Name] {
				if queued := queuedRound(dir); queued != "" {
					r.Message = queued
					r.DeliveryStatus = "queued for next round"
				}
			}
			return r, closeErr
		}
		unlock()
		q.Action = "rework"
		if e := admitOrchestrator(q); e != nil {
			return r, e
		}
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
		if q.Action == "rework" && readJSON(filepath.Join(dir, "meta.json"), &m) == nil && m.Forward != nil {
			unlock()
			locked = false
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if originalAction == "retry" {
				q.Action = "retry"
			}
			return routeRPC(ctx, *m.Forward, q)
		}
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
		policy, err := targetPolicy()
		if err != nil {
			return r, fmt.Errorf("target admission config: %w", err)
		}
		cap := policy.Cap
		if q.Cap > 0 && q.Cap < cap {
			cap = q.Cap
		}
		if q.excludeFinished != "" {
			delete(live, q.excludeFinished)
		}
		if q.Action == "start" && q.Meta.Parent == "" {
			entries, err := queueEntries()
			if err != nil {
				return r, err
			}
			if q.Dequeue {
				if len(entries) == 0 || entries[0].Request.Name != q.Name {
					return r, fmt.Errorf("queue head changed")
				}
				exclude := q.excludeFinished
				q = entries[0].Request
				q.excludeFinished = exclude
				q.Dequeue = true
			} else if len(live) >= cap || len(entries) > 0 {
				return enqueue(q)
			}
		}
		if e = CheckCap(len(live), cap); e != nil {
			return r, e
		}
		if e = reserveOrchestratorBudget(q, live); e != nil {
			return r, e
		}
		if q.Action == "start" {
			// A start its own reservation cannot use releases it; one that
			// created its run directory consumes it.
			defer func() {
				if handleErr != nil && locked && !exists(dir) {
					releaseClassSlot(q.Name, q.Orchestrator)
				}
			}()
			var o Order
			o, e = Parse(q.Order)
			if e != nil {
				return r, e
			}
			if (o.Name != q.Name && q.Meta.Parent != o.Name) || q.Meta.Name != q.Name {
				return r, fmt.Errorf("order name mismatch")
			}
			if e = admitClass(policy.Classes, o.Class, q.Name, live); e != nil {
				return r, e
			}
			m = q.Meta
			if m.Sender == "" {
				m.Sender = LocalSender()
			}
			m.RequestedBy = m.Sender
			// Launch settings are resolved on this host; never persist sender-supplied values.
			m.LaunchEnv, m.LaunchResolved = nil, false
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
			// Occupancy reads the class from metadata, never the sender's copy.
			m.Class = o.Class
			m.Host = q.Meta.Host
			if m.Parent == "" {
				m.Branch = "d/" + q.Name
			}
			m.Start = time.Now().UTC()
			if e = os.Mkdir(dir, 0700); e != nil {
				if os.IsExist(e) {
					return r, fmt.Errorf("%w: %s", errRunExists, q.Name)
				}
				return r, fmt.Errorf("cannot create run directory: %w", e)
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
			if q.Dequeue {
				if e = os.Remove(queuePath(q.Name)); e != nil {
					return r, e
				}
			}
			_ = os.Remove(classReservationPath(q.Name))
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
			if e = admitTargetRepo(repo, m.Repo, m.Host); e != nil {
				return r, e
			}
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
			if e = admitClass(policy.Classes, m.Class, q.Name, live); e != nil {
				return r, e
			}
			if _, e = os.Stat(filepath.Join(dir, "wt")); e != nil {
				return r, fmt.Errorf("worktree was cleaned")
			}
			if e = admitTargetRepo(filepath.Join(dir, "wt"), m.Repo, m.Host); e != nil {
				return r, e
			}
			cfg, err := loadTarget(m.ConfigPath, m.Host)
			if err != nil && (!os.IsNotExist(err) || m.ConfigPath != "") {
				return r, err
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
			priorHistory := history(dir, m)
			currentOrder, err := os.ReadFile(filepath.Join(dir, "order.md"))
			if err != nil {
				return r, err
			}
			before := string(currentOrder)
			revised := before + "\n## Rework\n\n" + q.Note + "\n"
			if e = atomicOrder(dir, revised); e != nil {
				return r, e
			}
			priorHistory.Revisions = append(priorHistory.Revisions, Revision{Number: len(priorHistory.Revisions) + 1, Time: time.Now().UTC(), Order: revised, Diff: orderDiff(before, revised)})
			if e = WriteJSON(filepath.Join(dir, "history.json"), priorHistory); e != nil {
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
			resolveLaunch(&m, cfg)
			if m.Every == "" {
				m.Start = time.Now().UTC()
			}
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
		// Serialize cancellation with the final launch handoff.
		unlockLaunch, err := lockRoot()
		if err != nil {
			return r, err
		}
		defer unlockLaunch()
		if q.Action == "start" && exists(filepath.Join(dir, "stop")) {
			r.Message = "cancelled " + q.Name
			return r, nil
		}
		_ = os.Remove(filepath.Join(dir, "stop"))
		_ = os.Remove(filepath.Join(dir, "supervisor.json"))
		if e = WriteJSON(filepath.Join(dir, "meta.json"), m); e != nil {
			return r, e
		}
		h := history(dir, m)
		if q.Action == "rework" {
			h.Attempts = append(h.Attempts, Attempt{Number: len(h.Attempts) + 1, Revision: len(h.Revisions), Meta: m})
		}
		if e = WriteJSON(filepath.Join(dir, "history.json"), h); e != nil {
			return r, e
		}
		if e = runLauncher(dir, q.Name); e != nil {
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
		h := history(dir, m)
		r.History = &h
		r.Active = live[q.Name]
		var pending Result
		if readJSON(filepath.Join(dir, "result.json"), &pending) == nil {
			reconcileLaunch(dir, &pending)
		}
		r.Files = map[string]string{}
		for _, f := range []string{"meta.json", "order.md", "status.md", "report.md", "result.json", "inbox.md", "validation.json", "verdict.json", "schedule.json", "round-notes.md"} {
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
			r.Message = "hev traces show " + result.Session
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
		unlockStop, err := lockRoot()
		if err != nil {
			return r, err
		}
		defer unlockStop()
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
