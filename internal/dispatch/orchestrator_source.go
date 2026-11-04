package dispatch

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

type SourceRule struct {
	Status   string `yaml:"status" json:"status"`
	Action   string `yaml:"action" json:"action"`
	Template string `yaml:"template" json:"template"`
	Mode     string `yaml:"mode" json:"mode"`
}
type OrchestratorSource struct {
	Plugin string       `yaml:"plugin,omitempty" json:"plugin,omitempty"`
	Team   string       `yaml:"team,omitempty" json:"team,omitempty"`
	Rules  []SourceRule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

func (s *OrchestratorSource) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		if n.Value == "none" {
			*s = OrchestratorSource{}
			return nil
		}
		return fmt.Errorf("source must be none or a mapping")
	}
	type plain OrchestratorSource
	b, e := yaml.Marshal(n)
	if e != nil {
		return e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	return d.Decode((*plain)(s))
}
func (s OrchestratorSource) Validate() error {
	if s.Plugin == "" {
		if len(s.Rules) > 0 || s.Team != "" {
			return fmt.Errorf("source plugin required")
		}
		return nil
	}
	if !ValidName(s.Plugin) {
		return fmt.Errorf("invalid source plugin")
	}
	seen := map[string]bool{}
	for _, r := range s.Rules {
		if r.Status == "" || !ValidName(r.Action) || r.Template == "" || (r.Mode != "live" && r.Mode != "dry-run") {
			return fmt.Errorf("source rule requires status, action, template and live|dry-run mode")
		}
		if seen[r.Status] {
			return fmt.Errorf("duplicate source status %s", r.Status)
		}
		seen[r.Status] = true
	}
	return nil
}

type SourceEvent struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	URL    string         `json:"url"`
	Fields map[string]any `json:"fields"`
}
type SourceBatch struct {
	Events []SourceEvent `json:"events"`
	Cursor string        `json:"cursor"`
	Ack    bool          `json:"ack"`
}

// sourceCommand invokes the public plugin entry point, never a private intake.
var sourceCommand = func(ctx context.Context, plugin string, args ...string) ([]byte, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, e
	}
	return exec.CommandContext(ctx, exe, append([]string{plugin}, args...)...).Output()
}

func fetchSource(ctx context.Context, s OrchestratorSpec) (SourceBatch, error) {
	var batch SourceBatch
	b, e := sourceCommand(ctx, s.Source.Plugin, "--hev-describe")
	if e != nil {
		return batch, e
	}
	var desc struct {
		Source bool `json:"source"`
		Ack    bool `json:"ack"`
	}
	if e = json.Unmarshal(b, &desc); e != nil {
		return batch, e
	}
	if !desc.Source {
		return batch, fmt.Errorf("plugin does not advertise source support")
	}
	batch.Ack = desc.Ack
	var cursor struct {
		Cursor string `json:"cursor"`
	}
	path := filepath.Join(OrchestratorDir(s.Name), "source-cursor.json")
	if e = readJSON(path, &cursor); e != nil && !os.IsNotExist(e) {
		return batch, e
	}
	args := []string{"events", "--since", cursor.Cursor}
	if s.Source.Team != "" {
		args = append(args, "--team", s.Source.Team)
	}
	b, e = sourceCommand(ctx, s.Source.Plugin, args...)
	if e != nil {
		return batch, e
	}
	scan := bufio.NewScanner(bytes.NewReader(b))
	scan.Buffer(make([]byte, 4096), 4*1024*1024)
	terminal := false
	ids := map[string]bool{}
	for scan.Scan() {
		if terminal {
			return batch, fmt.Errorf("source data after cursor")
		}
		var raw map[string]json.RawMessage
		if e = json.Unmarshal(scan.Bytes(), &raw); e != nil {
			return batch, e
		}
		if _, ok := raw["cursor"]; ok {
			if len(raw) != 1 {
				return batch, fmt.Errorf("cursor must be a separate final record")
			}
			if e = json.Unmarshal(raw["cursor"], &batch.Cursor); e != nil {
				return batch, e
			}
			terminal = true
			continue
		}
		if _, ok := raw["url"]; !ok {
			return batch, fmt.Errorf("source event missing url")
		}
		var event SourceEvent
		if e = json.Unmarshal(scan.Bytes(), &event); e != nil {
			return batch, e
		}
		if event.ID == "" || event.Kind == "" || event.Fields == nil || ids[event.ID] {
			return batch, fmt.Errorf("invalid or duplicate source event")
		}
		ids[event.ID] = true
		batch.Events = append(batch.Events, event)
	}
	if e = scan.Err(); e != nil {
		return batch, e
	}
	if !terminal {
		return batch, fmt.Errorf("source missing final cursor")
	}
	return batch, nil
}
func sourcePaused(s OrchestratorSpec) bool {
	if s.PauseFile == "" {
		return false
	}
	path := s.PauseFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(OrchestratorDir(s.Name), path)
	}
	return exists(path)
}

var sourceDispatch = RPC

// Markers are receipts for attempts, including uncertain transport outcomes.
// Never blindly retry an uncertain dispatch. Inspect it before removing a marker.
func runSourceRules(ctx context.Context, s OrchestratorSpec, dir string, batch SourceBatch, ev []OrchestratorEvidence) error {
	if e := os.WriteFile(filepath.Join(dir, "trace"), []byte("agent=off source="+s.Source.Plugin+"\n"), 0600); e != nil {
		return e
	}
	if sourcePaused(s) {
		return fmt.Errorf("source pause_file is present")
	}
	cfg, e := Load(ConfigPath(""))
	if e != nil {
		return e
	}
	active := 0
	for _, v := range ev {
		if v.Row.State == "running" {
			active++
		}
	}
	var budget struct {
		Day  string `json:"day"`
		Runs int    `json:"runs"`
	}
	path := filepath.Join(OrchestratorDir(s.Name), "source-budget.json")
	if e = readJSON(path, &budget); e != nil && !os.IsNotExist(e) {
		return e
	}
	day := time.Now().UTC().Format("2006-01-02")
	if budget.Day != day {
		budget.Day = day
		budget.Runs = 0
	}
	for _, event := range batch.Events {
		for _, rule := range s.Source.Rules {
			if event.Fields["status"] != rule.Status {
				continue
			}
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(s.Source.Plugin+"\x00"+event.ID+"\x00"+rule.Action+"\x00"+rule.Mode)))
			markers := filepath.Join(OrchestratorDir(s.Name), "source-markers")
			marker := filepath.Join(markers, key+".json")
			if exists(marker) {
				var receipt struct {
					Mode string `json:"mode"`
				}
				if e := readJSON(marker, &receipt); e != nil {
					return e
				}
				if receipt.Mode == "attempting" {
					return fmt.Errorf("source dispatch has an uncertain receipt %s; inspect before clearing marker", key)
				}
				continue
			}
			if sourcePaused(s) {
				return fmt.Errorf("source pause_file is present")
			}
			if rule.Mode == "live" && ((s.Budget.ConcurrentRuns > 0 && active >= s.Budget.ConcurrentRuns) || (s.Budget.RunsPerDay > 0 && budget.Runs >= s.Budget.RunsPerDay)) {
				return fmt.Errorf("source run cap reached; events remain pending")
			}
			tplPath := rule.Template
			if !filepath.IsAbs(tplPath) {
				tplPath = filepath.Join(OrchestratorDir(s.Name), tplPath)
			}
			raw, e := os.ReadFile(tplPath)
			if e != nil {
				return e
			}
			tpl, e := template.New("order").Option("missingkey=error").Parse(string(raw))
			if e != nil {
				return e
			}
			var rendered bytes.Buffer
			name := "src-" + s.Name + "-" + key[:16]
			if e = tpl.Execute(&rendered, struct {
				Event           SourceEvent
				Action, RunName string
			}{event, rule.Action, name}); e != nil {
				return e
			}
			order, e := Parse(rendered.String())
			if e != nil {
				return e
			}
			if order.Name != name || !s.Includes(order) {
				return fmt.Errorf("source template must use RunName and remain in manages scope")
			}
			if e = os.WriteFile(filepath.Join(dir, name+".md"), rendered.Bytes(), 0600); e != nil {
				return e
			}
			if e = os.MkdirAll(markers, 0700); e != nil {
				return e
			}
			if rule.Mode == "dry-run" {
				if e = WriteJSON(marker, map[string]any{"mode": "dry-run", "event": event.ID, "order": rendered.String()}); e != nil {
					return e
				}
				continue
			}
			host := order.Host
			if host == "" {
				host = cfg.DefaultHost
			}
			h, ok := cfg.Hosts[host]
			if !ok {
				return fmt.Errorf("unknown source target %s", host)
			}
			q := Request{Action: "start", Name: name, Order: rendered.String(), Orchestrator: s.Name, Meta: Meta{Order: order, Namespace: h.Namespace, HarnessWrapper: h.HarnessWrapper, ConfigPath: h.ConfigPath}, Cap: h.Cap, Workspace: h.Workspace}
			check := q
			check.AdmissionOnly = true
			if _, e = sourceDispatch(ctx, h, check); e != nil {
				return e
			}
			budget.Runs++
			if e = WriteJSON(path, budget); e != nil {
				return e
			}
			if e = WriteJSON(marker, map[string]string{"mode": "attempting", "event": event.ID, "run": name}); e != nil {
				return e
			}
			if _, e = sourceDispatch(ctx, h, q); e != nil {
				return fmt.Errorf("source dispatch %s uncertain; inspect before clearing marker: %w", name, e)
			}
			if e = WriteJSON(marker, map[string]string{"mode": "live", "event": event.ID, "run": name}); e != nil {
				return e
			}
			active++
		}
	}
	return os.WriteFile(filepath.Join(dir, "checkpoint.md"), []byte("Source events processed. Next: poll from accepted cursor.\n"), 0600)
}

// A model round may choose whether to act, but cannot promote a dry-run rule,
// invent an event, or change a rule's rendered order. Dispatch admission owns
// the live caps and the deterministic name makes duplicate starts reject.
func admitSourceOrder(s OrchestratorSpec, q Request) error {
	if q.Action != "list" && q.Action != "show" && q.Action != "logs" && sourcePaused(s) {
		return fmt.Errorf("source pause_file is present")
	}
	if q.Action != "start" || q.Meta.Parent != "" || s.Source.Plugin == "" || len(s.Source.Rules) == 0 {
		return nil
	}
	st, e := orchestratorState(s.Name)
	if e != nil {
		return e
	}
	var packet OrchestratorPacket
	if e = readJSON(filepath.Join(OrchestratorDir(s.Name), "rounds", fmt.Sprintf("%03d", st.Rounds), "packet.json"), &packet); e != nil {
		return e
	}
	for _, event := range packet.Source.Events {
		for _, rule := range s.Source.Rules {
			if rule.Mode != "live" || event.Fields["status"] != rule.Status {
				continue
			}
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(s.Source.Plugin+"\x00"+event.ID+"\x00"+rule.Action+"\x00"+rule.Mode)))
			name := "src-" + s.Name + "-" + key[:16]
			if q.Name != name {
				continue
			}
			path := rule.Template
			if !filepath.IsAbs(path) {
				path = filepath.Join(OrchestratorDir(s.Name), path)
			}
			raw, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			tpl, e := template.New("order").Option("missingkey=error").Parse(string(raw))
			if e != nil {
				return e
			}
			var rendered bytes.Buffer
			if e = tpl.Execute(&rendered, struct {
				Event           SourceEvent
				Action, RunName string
			}{event, rule.Action, name}); e != nil {
				return e
			}
			if strings.TrimSpace(q.Order) == strings.TrimSpace(rendered.String()) {
				return nil
			}
		}
	}
	return fmt.Errorf("source order does not match a live rule in the current packet")
}
