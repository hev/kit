package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/workspace"
)

type pickerPane struct {
	ID, Name, Dir, Workspace, Project, Role    string
	Pane, Harness, State, Title, Prompt, Trace string
}

type pickerSnapshot struct {
	Panes []pickerPane
	Hosts []cluster.Member
	Peers int
}

func pickerCachePath() string {
	socket := os.Getenv("HEV_TMUX_SOCKET")
	if socket == "" {
		socket, _, _ = strings.Cut(os.Getenv("TMUX"), ",")
	}
	if socket == "" {
		socket = filepath.Join(os.Getenv("TMUX_TMPDIR"), "default")
	}
	config, _ := filepath.Abs(layerenv.ConfigPath())
	key := sha256.Sum256([]byte(config + "\x00" + socket))
	return filepath.Join(xdgPath("XDG_CACHE_HOME", "~/.cache"), "hev", "pick", fmt.Sprintf("%x.json", key[:12]))
}

func readPickerSnapshot() pickerSnapshot {
	var snapshot pickerSnapshot
	if raw, err := os.ReadFile(pickerCachePath()); err == nil {
		_ = json.Unmarshal(raw, &snapshot)
	}
	return snapshot
}

func refreshPicker(cfg workspace.Config) error {
	path := pickerCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if stat, err := os.Stat(path); err == nil && time.Since(stat.ModTime()) < 2*time.Second {
		return nil
	}
	snapshot := readPickerSnapshot()
	snapshot.Panes = livePickerPanes()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	peers := exec.CommandContext(ctx, executable, "peers-native", "ls", "--json")
	// An explicit socket may have been selected outside tmux.
	if socket := os.Getenv("HEV_TMUX_SOCKET"); socket != "" {
		if actual, err := tmuxOutput("display-message", "-p", "#{socket_path}"); err == nil && actual != "" {
			peers.Env = append(os.Environ(), "TMUX="+actual+",0,0")
		}
	}
	status := exec.CommandContext(ctx, executable, "cluster", "ls", "--json")
	if !cfg.Pick.HideBOH {
		var hosts []cluster.Member
		if raw, err := status.Output(); err == nil && json.Unmarshal(raw, &hosts) == nil {
			snapshot.Hosts = hosts
		}
	}
	type peer struct {
		Pane, Agent, State string
		Trace              string `json:"session_id"`
	}
	var observed struct{ Panes []peer }
	if raw, err := peers.Output(); err == nil && json.Unmarshal(raw, &observed) == nil {
		rows := make(map[string]peer)
		for _, p := range observed.Panes {
			if p.Agent != "" {
				rows[p.Pane] = p
			}
		}
		snapshot.Peers = 0
		for i := range snapshot.Panes {
			row := &snapshot.Panes[i]
			if p, ok := rows[row.Pane]; ok {
				row.Harness, row.State = p.Agent, p.State
				if p.Trace != "" {
					row.Trace = p.Trace
				}
				snapshot.Peers++
			}
		}
	}
	// Publish the live list before the archive can delay metadata enrichment.
	previous := readPickerSnapshot()
	prior := make(map[string]pickerPane)
	for _, pane := range previous.Panes {
		if pane.Trace != "" {
			prior[pane.Trace] = pane
		}
	}
	for i := range snapshot.Panes {
		pane := &snapshot.Panes[i]
		if old, ok := prior[pane.Trace]; ok {
			pane.Title, pane.Prompt = old.Title, old.Prompt
		}
	}
	if err := writePickerSnapshot(path, snapshot); err != nil {
		return err
	}
	raw, err := exec.CommandContext(ctx, executable, "pick", "--trace-details").Output()
	if err != nil {
		return nil
	}
	var details map[string]pickerTraceDetails
	if json.Unmarshal(raw, &details) != nil {
		return nil
	}
	for i := range snapshot.Panes {
		p := &snapshot.Panes[i]
		if detail, ok := details[p.Trace]; ok {
			p.Title, p.Prompt = detail.Title, detail.Prompt
		}
	}
	return writePickerSnapshot(path, snapshot)
}

func livePickerPanes() []pickerPane {
	format := strings.Join([]string{"#{session_id}", "#{session_name}", "#{pane_current_path}", "#{@hev-workspace}", "#{@hev-project}", "#{@peers-role}", "#{pane_id}", "#{@hev-harness}", "#{@hev-trace}", "#{session_activity}"}, "\t")
	out, _ := tmuxOutput("list-panes", "-a", "-F", format)
	lines := strings.Split(out, "\n")
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i][strings.LastIndex(lines[i], "\t")+1:] > lines[j][strings.LastIndex(lines[j], "\t")+1:]
	})
	var panes []pickerPane
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) == 10 {
			panes = append(panes, pickerPane{ID: f[0], Name: f[1], Dir: f[2], Workspace: f[3], Project: f[4], Role: f[5], Pane: f[6], Harness: f[7], Trace: f[8]})
		}
	}
	return panes
}

func writePickerSnapshot(path string, snapshot pickerSnapshot) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".pick-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	encodeErr := json.NewEncoder(file).Encode(snapshot)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

type pickerTraceDetails struct{ Title, Prompt string }

func pickTraceDetails(cmdOut *json.Encoder) error {
	snapshot := readPickerSnapshot()
	details := make(map[string]pickerTraceDetails)
	haveTrace := false
	for _, p := range snapshot.Panes {
		haveTrace = haveTrace || p.Trace != ""
	}
	if !haveTrace {
		return cmdOut.Encode(details)
	}
	store, err := traces("")
	if err != nil {
		return err
	}
	sessions, err := store.ListSessionRows(500, []any{"start", "Gte", time.Now().AddDate(0, 0, -14).UnixMilli()})
	if err != nil {
		return err
	}
	wanted := make(map[string]bool)
	for _, p := range snapshot.Panes {
		if p.Trace != "" {
			wanted[p.Trace] = true
		}
	}
	for _, s := range sessions {
		if !wanted[s.SessionID] {
			continue
		}
		detail := pickerTraceDetails{Title: s.Summary, Prompt: s.FirstPromptShort}
		if rows, err := store.SessionRows(s.SessionID); err == nil {
			for i := len(rows) - 1; i >= 0; i-- {
				if rows[i].Role == "user" && strings.TrimSpace(rows[i].Text) != "" {
					detail.Prompt = rows[i].Text
					break
				}
			}
		}
		details[s.SessionID] = detail
	}
	return cmdOut.Encode(details)
}
