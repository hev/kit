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
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/workspace"
)

type pickerPane struct {
	ID, Name, Dir, Workspace, Project, Role    string
	Pane, Harness, State, Title, Prompt, Trace string
	AgentStarted                               int64 `json:"-"`
	Active                                     bool
}

type pickerSnapshot struct {
	Panes   []pickerPane
	Hosts   []pickerHost
	History []pickerConversation
	Peers   int
}

type pickerHost struct {
	Name, OS, State, Activity string
}

type pickerConversation struct {
	ID, Harness, Dir, Title, Prompt string
}

func pickerCachePath() string {
	if path := os.Getenv("HEV_PICK_CACHE"); path != "" {
		return path
	}
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
	// fzf may request the list and footer at the same time. Its normal path
	// calls refresh once, and atomic replacement keeps readers independent.
	snapshot := readPickerSnapshot()
	previous := snapshot.Panes
	snapshot.Panes = livePickerPanes()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	peers := exec.CommandContext(ctx, executable, "peers-native", "ls", "--json")
	if os.Getenv("HEV_TMUX_SOCKET") != "" {
		if actual, err := tmuxOutput("display-message", "-p", "#{socket_path}"); err == nil && actual != "" {
			peers.Env = append(os.Environ(), "TMUX="+actual+",0,0")
		}
	}
	var hostResults chan []pickerHost
	if !cfg.Pick.HideBOH {
		hostResults = make(chan []pickerHost, 1)
		go func() {
			var members []cluster.Member
			var hosts []pickerHost
			status := exec.CommandContext(ctx, executable, "cluster", "ls", "--json")
			if raw, err := status.Output(); err == nil && json.Unmarshal(raw, &members) == nil {
				hosts = []pickerHost{}
				for _, m := range members {
					if !m.Self {
						hosts = append(hosts, pickerHost{m.Name, m.OS, m.State, m.Activity.Short()})
					}
				}
			}
			hostResults <- hosts
		}()
	}
	type peer struct {
		Pane, Agent, State, Cwd string
		Trace                   string `json:"session_id"`
		Started                 int64  `json:"agent_started"`
	}
	var observed struct{ Panes []peer }
	if raw, err := peers.Output(); err == nil && json.Unmarshal(raw, &observed) == nil {
		rows := make(map[string]peer)
		snapshot.Peers = 0
		for _, p := range observed.Panes {
			if p.Agent != "" {
				rows[p.Pane] = p
				snapshot.Peers++
			}
		}
		for i := range snapshot.Panes {
			row := &snapshot.Panes[i]
			if p, ok := rows[row.Pane]; ok {
				row.Harness, row.State, row.AgentStarted = p.Agent, p.State, p.Started
				if p.Trace != "" {
					row.Trace = p.Trace
				}
			}
		}
	}
	if hostResults != nil {
		if hosts := <-hostResults; hosts != nil {
			snapshot.Hosts = hosts
		}
	}
	prior := make(map[string]pickerPane)
	for _, p := range previous {
		prior[p.Pane] = p
	}
	for i := range snapshot.Panes {
		p := &snapshot.Panes[i]
		old, ok := prior[p.Pane]
		if ok && p.ID == old.ID && p.Trace != "" && p.Trace == old.Trace {
			p.Title, p.Prompt = old.Title, old.Prompt
		}
	}
	enrichPicker(&snapshot)
	return writePickerSnapshot(path, snapshot)
}

func livePickerPanes() []pickerPane {
	format := strings.Join([]string{"#{session_id}", "#{session_name}", "#{pane_current_path}", "#{@hev-workspace}", "#{@hev-project}", "#{@peers-role}", "#{pane_id}", "#{@hev-harness}", "#{@hev-trace}", "#{window_active}#{pane_active}", "#{session_activity}"}, "\t")
	out, _ := tmuxOutput("list-panes", "-a", "-F", format)
	lines := strings.Split(out, "\n")
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i][strings.LastIndex(lines[i], "\t")+1:] > lines[j][strings.LastIndex(lines[j], "\t")+1:]
	})
	var panes []pickerPane
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) == 11 {
			panes = append(panes, pickerPane{ID: f[0], Name: f[1], Dir: f[2], Workspace: f[3], Project: f[4], Role: f[5], Pane: f[6], Harness: f[7], Trace: f[8], Active: f[9] == "11"})
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

// Exact trace IDs take precedence. For harnesses whose peers observation
// has no ID, require a unique archive start near the agent process start in
// the same directory and harness. Ambiguous and resumed agents may be blank.
func matchPickerTrace(p pickerPane, sessions []trace.SessionRow) *trace.SessionRow {
	var match *trace.SessionRow
	for i := range sessions {
		row := &sessions[i]
		if p.Trace != "" {
			if row.SessionID == p.Trace {
				return row
			}
			continue
		}
		age := row.Start - p.AgentStarted*1000
		if p.AgentStarted == 0 || p.Harness != row.Harness || p.Dir != row.Workdir || age < -2000 || age > 120000 {
			continue
		}
		if match != nil {
			return nil
		}
		match = row
	}
	return match
}

func enrichPicker(snapshot *pickerSnapshot) {
	store, err := traces("")
	if err != nil {
		return
	}
	sessions, err := store.ListSessionRows(500, []any{"start", "Gte", time.Now().AddDate(0, 0, -14).UnixMilli()})
	if err != nil {
		return
	}
	history := make([]pickerConversation, 0, len(sessions))
	for _, s := range sessions {
		history = append(history, pickerConversation{s.SessionID, s.Harness, s.Workdir, s.Summary, s.FirstPromptShort})
	}
	snapshot.History = history
	for i := range snapshot.Panes {
		p := &snapshot.Panes[i]
		s := matchPickerTrace(*p, sessions)
		if s == nil {
			continue
		}
		p.Trace, p.Title, p.Prompt = s.SessionID, s.Summary, s.FirstPromptShort
		if rows, err := store.SessionRows(s.SessionID); err == nil {
			for i := len(rows) - 1; i >= 0; i-- {
				if rows[i].Role == "user" && strings.TrimSpace(rows[i].Text) != "" {
					p.Prompt = rows[i].Text
					break
				}
			}
		}
		p.Prompt = strings.ToValidUTF8(p.Prompt[:min(500, len(p.Prompt))], "")
	}
}
