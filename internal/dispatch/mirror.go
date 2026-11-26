package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/workspace"
	"github.com/hev/kit/pkg/kitapi"
)

type mirrorSignIn workspace.KitBinding

func orderMirrorSignIn(name string) (mirrorSignIn, bool) {
	if os.Getenv("HEV_KIT_MIRROR_ORDERS") == "0" {
		return mirrorSignIn{}, false
	}
	var config struct {
		Kit        mirrorSignIn
		Workspaces map[string]struct {
			Kit      *mirrorSignIn
			Archived bool
		}
	}
	if _, err := toml.DecodeFile(workspace.ConfigPath(), &config); err != nil {
		return mirrorSignIn{}, false
	}
	name = workspaceOfRun(name)
	w, exists := config.Workspaces[name]
	if w.Archived || name != workspace.Default && (!exists || w.Kit == nil) {
		return mirrorSignIn{}, false
	}
	signIn := config.Kit
	if w.Kit != nil {
		signIn = *w.Kit
	}
	return signIn, kitapi.ValidBaseURL(signIn.API) && kitapi.ValidKey(signIn.Key) &&
		(strings.HasPrefix(signIn.Key, "hev_") || strings.HasPrefix(signIn.Key, "hk_"))
}

func (s mirrorSignIn) fingerprint() [32]byte {
	return sha256.Sum256([]byte(s.API + "\x00" + s.Key))
}

func mirrorMutation(q Request) bool {
	if q.AdmissionOnly || q.APILocalOnly || q.Dequeue {
		return false
	}
	switch q.Action {
	case "start", "rework", "retry", "say", "stop", "close":
		return true
	}
	return false
}

var (
	mirrorPath   = regexp.MustCompile(`(?i)(?:op://|file:/+|[a-z]:[/\\]|~[/\\]|\\\\|(?:^|[\s"'=:(])[/\\])[^\s"'<>]*`)
	mirrorRef    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]{0,199}$`)
	mirrorRepo   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,99}(?:/[A-Za-z0-9_][A-Za-z0-9_.-]{0,99})?$`)
	mirrorPR     = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]{1,12}$`)
	mirrorSecret = regexp.MustCompile(`(?i)(?:hev_|hvl_|hk_|sk-or-v1-)[a-z0-9_-]{16,}`)
	mirrorMarker = regexp.MustCompile(`\[REDACTED:[a-zA-Z0-9_-]+#[0-9a-f]{16}\]`)
	mirrorID     = regexp.MustCompile(`^[0-9a-f]{16,64}$`)
)

func cleanMirrorText(scrubber *redact.Scrubber, text, key string) string {
	text = mirrorSecret.ReplaceAllString(redact.HideKitKeys(text, key), "[redacted]")
	text = mirrorPath.ReplaceAllString(text, " [redacted]")
	text, _ = scrubber.Text(text)
	return strings.Join(strings.Fields(mirrorMarker.ReplaceAllString(text, "[redacted]")), " ")
}

func mirrorField(text string, pattern *regexp.Regexp, scrubber *redact.Scrubber) string {
	if pattern.MatchString(text) && !strings.Contains(text, "..") && cleanMirrorText(scrubber, text, "") == text {
		return text
	}
	return ""
}

func mirrorOrder(m Meta, result Result, state, title string, updated time.Time) *kitapi.MirroredOrder {
	scrubber, err := redact.New([]byte("hev dispatch mirrored order metadata"))
	if err != nil {
		return nil
	}
	name, host := mirrorField(m.Name, namePattern, scrubber), mirrorField(m.Host, namePattern, scrubber)
	if name == "" || host == "" || len(name) > 128 || len(host) > 128 || m.Start.IsZero() {
		return nil
	}
	decision := result.Question
	if result.ClosedReason != "" {
		decision = result.ClosedReason
	} else if decision == "" {
		decision = result.Reason
	}
	if len(decision) > 512 {
		decision = ""
	}
	return &kitapi.MirroredOrder{
		Name: name, Host: host, Repo: mirrorField(m.Repo, mirrorRepo, scrubber),
		Base: mirrorField(m.Base, mirrorRef, scrubber), Branch: mirrorField(m.Branch, mirrorRef, scrubber),
		Harness: mirrorField(m.Harness, namePattern, scrubber), State: state,
		Title: cleanMirrorText(scrubber, title, ""), Decision: cleanMirrorText(scrubber, decision, ""),
		Error: cleanMirrorText(scrubber, result.Error, ""), PRURL: mirrorURL(result.PR, scrubber),
		TraceID: mirrorField(m.TraceID, mirrorID, scrubber), StartedAt: m.Start, EndedAt: result.End, UpdatedAt: updated,
	}
}

func mirrorURL(raw string, scrubber *redact.Scrubber) string {
	if !mirrorPR.MatchString(raw) || cleanMirrorText(scrubber, strings.TrimPrefix(raw, "https://github.com/"), "") != strings.TrimPrefix(raw, "https://github.com/") {
		return ""
	}
	return raw
}

func mirrorTitle(dir string) string {
	f, err := os.Open(filepath.Join(dir, "order.md"))
	if err != nil {
		return ""
	}
	defer f.Close()
	var raw [8192]byte
	n, _ := f.Read(raw[:])
	return mirrorOrderTitle(string(raw[:n]))
}

func mirrorOrderTitle(text string) string {
	if rest, ok := strings.CutPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "---\n"); ok {
		_, body, hasBody := strings.Cut(rest, "\n---\n")
		if !hasBody {
			return ""
		}
		text = body
	}
	for _, line := range strings.Split(text, "\n") {
		if title := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#")); title != "" {
			if len(title) <= 512 {
				return title
			}
			return ""
		}
	}
	return ""
}

func queuedMirror(entry queueEntry, state string, update time.Time) *kitapi.MirroredOrder {
	var result Result
	if state == "halted" {
		result.End = update
	}
	return mirrorOrder(entry.Request.Meta, result, state, mirrorOrderTitle(entry.Request.Order), update)
}

func readOrderMirror(name, selected string) *kitapi.MirroredOrder {
	dir := filepath.Join(Root(), name)
	var m Meta
	if readJSON(filepath.Join(dir, "meta.json"), &m) != nil {
		var entry queueEntry
		if readJSON(queuePath(name), &entry) == nil && checkRunWorkspace(selected, entry.Request.Meta) == nil {
			return queuedMirror(entry, "queued", entry.Enqueued)
		}
		return nil
	}
	if m.Forward != nil || checkRunWorkspace(selected, m) != nil {
		return nil
	}
	var result Result
	_ = readJSON(filepath.Join(dir, "result.json"), &result)
	if !result.Start.IsZero() && result.Start.Before(m.Start) {
		result = Result{}
	}
	if len(result.Question) > 512 || len(result.Reason) > 512 {
		result.Question, result.Reason = "", ""
	}
	if len(result.Error) > 512 {
		result.Error = ""
	}
	// The launcher can return before the supervisor has written its pid.
	// Give a newly admitted attempt the same grace as an unconfirmed launch.
	active := result.End.IsZero() && !result.Closed() &&
		(supervisorLive(dir) || processRecordLive(filepath.Join(dir, "admission.json")) ||
			(!result.LaunchUnconfirmed && time.Since(m.Start) < launchGrace))
	if result.LaunchUnconfirmed && !active && time.Since(result.Start) >= launchGrace {
		result.LaunchUnconfirmed = false
	}
	state, stamp := State(&result, active), m.Start
	switch {
	case result.Closed():
		stamp = result.ClosedAt
		if result.End.IsZero() {
			result.End = stamp
		}
	case !result.End.IsZero():
		stamp = result.End
		if result.Stopped {
			state = "halted"
		}
	case exists(filepath.Join(dir, "stop")):
		if file, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			state, stamp = "halting", file.ModTime()
		}
	case state == "died":
		// There may be no result file when a supervisor is killed.
		result.End = time.Now().UTC()
		stamp = result.End
	}
	return mirrorOrder(m, result, state, mirrorTitle(dir), stamp)
}

type mirrorWatch struct {
	Host      Host
	Workspace string
	Order     kitapi.MirroredOrder
	Watch     bool
	Binding   [32]byte
}

var launchOrderMirror = startOrderMirror

func startOrderMirror(host Host, selected string, order *kitapi.MirroredOrder, watch bool, signIn mirrorSignIn) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	route := Host{Local: host.Local, SSH: host.SSH, OS: host.OS, PathPrefix: host.PathPrefix}
	payload, err := json.Marshal(mirrorWatch{route, selected, *order, watch, signIn.fingerprint()})
	if err != nil {
		return
	}
	input, err := os.CreateTemp("", "hev-order-mirror-*")
	if err != nil {
		mirrorDebug()
		return
	}
	defer input.Close()
	defer os.Remove(input.Name())
	if _, err := input.Write(payload); err != nil {
		mirrorDebug()
		return
	}
	if _, err := input.Seek(0, 0); err != nil {
		mirrorDebug()
		return
	}
	cmd := exec.Command(exe, "dispatch", "mirror-order")
	cmd.Stdin = input
	if os.Getenv("HEV_KIT_MIRROR_DEBUG") == "1" {
		if info, err := os.Stderr.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			cmd.Stderr = os.Stderr
		}
	}
	if err := startDetached(cmd); err != nil {
		mirrorDebug()
	}
}

func mirrorDebug() {
	if os.Getenv("HEV_KIT_MIRROR_DEBUG") == "1" {
		fmt.Fprintln(os.Stderr, "dispatch: hosted order mirror update unavailable")
	}
}

func mirrorActive(state string) bool {
	switch state {
	case "running", "queued", "halting", "launch-unconfirmed":
		return true
	}
	return false
}

// RunOrderMirror runs outside the dispatching process. The SSH target only
// returns snapshots and never receives the initiating workspace's kit key.
func RunOrderMirror(input io.Reader) {
	var watch mirrorWatch
	if json.NewDecoder(io.LimitReader(input, 64<<10)).Decode(&watch) != nil || !ValidName(watch.Order.Name) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*24*time.Hour)
	defer cancel()
	watch.run(ctx, 30*time.Second, func(ctx context.Context) (*kitapi.MirroredOrder, bool) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		r, err := rpcTransport(ctx, watch.Host, Request{Action: "mirror-snapshot", Name: watch.Order.Name, WorkspaceName: watch.Workspace})
		if err != nil {
			return nil, false
		}
		return r.MirroredOrder, true
	})
}

func (w mirrorWatch) run(ctx context.Context, interval time.Duration, snapshot func(context.Context) (*kitapi.MirroredOrder, bool)) {
	order := &w.Order
	var sent kitapi.MirroredOrder
	scrubber, err := redact.New([]byte("hev dispatch mirrored order metadata"))
	if err != nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	missing := 0
	for {
		signIn, ok := orderMirrorSignIn(w.Workspace)
		if !ok || signIn.fingerprint() != w.Binding {
			return
		}
		if order != nil {
			if order.Name != w.Order.Name || order.Host != w.Order.Host ||
				(!order.StartedAt.Equal(w.Order.StartedAt) && w.Order.State != "queued") {
				return // a newer attempt has its own publisher
			}
			safe := *order
			for _, field := range []*string{&safe.Title, &safe.Decision, &safe.Error} {
				*field = cleanMirrorText(scrubber, *field, signIn.Key)
			}
			if sent != safe {
				if err := kitapi.New(signIn.API, signIn.Key).MirrorOrder(ctx, safe); err != nil {
					mirrorDebug()
				} else {
					sent = safe
				}
			}
			if !w.Watch || !mirrorActive(order.State) {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var reached bool
			order, reached = snapshot(ctx)
			if order != nil {
				missing = 0
			} else if reached {
				missing++
				if missing >= 3 {
					return
				}
			}
		}
	}
}
