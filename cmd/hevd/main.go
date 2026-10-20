// hevd accepts an explicit durable run. It never chooses or schedules work.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hev/factory/pkg/direct"
	"github.com/hev/kit/pkg/nativejob"
)

type config struct {
	Node        string   `json:"node"`
	Incarnation string   `json:"incarnation"`
	Journal     string   `json:"journal"`
	Transport   string   `json:"transport"`
	Codex       string   `json:"codex"`
	Models      []string `json:"models"`
}

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "hevd:", e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) < 1 {
		return errors.New("use execute|recover|observe --config FILE --run ID")
	}
	action := args[0]
	if action == "harness" {
		return harness(args[1:])
	}
	fs := flag.NewFlagSet("hevd", flag.ContinueOnError)
	cfgfile := fs.String("config", "", "Owner-selected executor configuration")
	runID := fs.String("run", "", "Exact durable run ID")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	b, err := os.ReadFile(*cfgfile)
	if err != nil {
		return err
	}
	var cfg config
	if err = json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	if cfg.Node == "" || cfg.Incarnation == "" || !filepath.IsAbs(cfg.Journal) || !filepath.IsAbs(cfg.Codex) {
		return errors.New("explicit node, incarnation, private journal and absolute codex required")
	}
	c := direct.Client{Transport: cfg.Transport}
	ctx := context.Background()
	if action == "observe" {
		_, err = c.Change(ctx, "observe", direct.Row{"id": cfg.Node, "actor": cfg.Node, "incarnation": cfg.Incarnation, "ttl": 120, "observation": direct.Row{"available": true, "os": hostOS(), "harnesses": []string{"codex"}, "models": cfg.Models, "capacity": 1}})
		return err
	}
	if action != "execute" && action != "recover" {
		return errors.New("unsupported execution operation")
	}
	rows, err := c.List(ctx, "runs", *runID)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return errors.New("run missing")
	}
	r := rows[0]
	if r["node_id"] != cfg.Node || r["incarnation"] != cfg.Incarnation {
		return errors.New("run is assigned to another execution identity")
	}
	a, ok := r["assignment"].(map[string]any)
	if !ok {
		return errors.New("assignment missing")
	}
	if err = direct.ValidateAssignment(a); err != nil {
		return err
	}
	model := a["model"].(string)
	allowed := false
	for _, m := range cfg.Models {
		if m == model {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("requested model unavailable; FOH must select an eligible node")
	}
	s, err := nativejob.Open(cfg.Journal, nativejob.Enrollment{HostID: cfg.Node, ExecutorID: cfg.Incarnation})
	if err != nil {
		return err
	}
	defer s.Close()
	key := r["id"].(string)
	publish := func(j nativejob.Job, inspection string) error {
		if j.State != "Completed" {
			return nil
		}
		actualCandidate := a["candidate"]
		head := exec.Command("git", "-C", a["directory"].(string), "rev-parse", "HEAD")
		if out, e := head.Output(); e == nil {
			actualCandidate = strings.TrimSpace(string(out))
		} else {
			return errors.New("completed source candidate unavailable")
		}
		status := "succeeded"
		if j.Result.ExitCode != 0 {
			status = "failed"
		}
		p := direct.Row{"order": r["order_id"], "node": cfg.Node, "incarnation": cfg.Incarnation, "run": key, "status": status, "inspection": inspection, "result": direct.Row{"candidate": actualCandidate, "native_job": j.ID, "result_digest": j.ResultDigest, "exit_code": j.Result.ExitCode, "artifacts": j.Result.Artifacts, "passed": false}}
		// Exiting zero is not acceptance. Validators supply a separately inspected
		// candidate-bound acceptance receipt; the daemon never infers it from prose.
		_, err := c.Change(ctx, "report", p)
		return err
	}
	j, queryErr := s.Query(key)
	if action == "recover" {
		if queryErr != nil {
			return errors.New("local journal missing; do not replay the run")
		}
		j, err = s.Recover(j.ID)
		if err != nil {
			return err
		}
		if j.State == "Uncertain" {
			_, err = c.Change(ctx, "report", direct.Row{"order": r["order_id"], "node": cfg.Node, "incarnation": cfg.Incarnation, "run": key, "status": "unknown", "result": direct.Row{"native_job": j.ID, "reason": "execution effects require inspection"}})
		} else {
			err = publish(j, "")
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(j)
	}
	if queryErr == nil {
		if j.State != "Pending" {
			if err = publish(j, ""); err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(j)
		}
	} else {
		// A cloud Running record with no local intent is ambiguous, never permission
		// to recreate lost local state.
		if r["status"] != "requested" {
			return errors.New("local intent missing for observed run; reconcile effects")
		}
		dir := a["directory"].(string)
		head := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
		out, e := head.Output()
		if e != nil || strings.TrimSpace(string(out)) != a["source_revision"] {
			return errors.New("source revision does not match assignment")
		}
		clean := exec.Command("git", "-C", dir, "status", "--porcelain")
		out, e = clean.Output()
		if e != nil || len(out) != 0 {
			return errors.New("source directory must be clean before execution")
		}
		created, e := time.Parse(time.RFC3339Nano, r["created"].(string))
		if e != nil {
			return e
		}
		expires := created.Add(time.Duration(a["max_seconds"].(float64)+120) * time.Second)
		if err = s.AdvanceFence(nativejob.Fence{OrderUID: key, ChefID: cfg.Node, Epoch: 1, Expires: expires}); err != nil {
			return err
		}
		self, e := os.Executable()
		if e != nil {
			return e
		}
		prompt := a["prompt"].(string) + "\nRequired skills: " + fmt.Sprint(a["skills"]) + "\nAcceptance: " + fmt.Sprint(a["acceptance"]) + "\nExact assigned source: " + a["source_revision"].(string) + "\nRun identity: " + key
		req := nativejob.Request{Version: nativejob.Version, IntentKey: key, Directory: dir, Environment: executionEnvironment(), Argv: []string{self, "harness", strconv.Itoa(int(a["max_seconds"].(float64))), cfg.Codex, "exec", "--model", model, "--dangerously-bypass-approvals-and-sandbox", prompt}, Identity: nativejob.Identity{OrderUID: key, ScopeDigest: nativejob.Digest(a), PlanDigest: nativejob.Digest(a), AuthorityDigest: nativejob.Digest(r), ExecutionEpoch: key, WorkflowUID: key, NodeID: cfg.Node, StepID: key, AttemptID: key, CandidateDigest: a["candidate"].(string), InputDigest: nativejob.Digest(a), EnvironmentDigest: nativejob.Digest(executionEnvironment()), Operation: "exec", HostID: cfg.Node, ExecutorID: cfg.Incarnation, ChefID: cfg.Node, Fence: 1}}
		req.PayloadDigest = nativejob.RequestDigest(req)
		j, err = s.Submit(req)
		if err != nil {
			return err
		}
	}
	_, err = c.Change(ctx, "start", direct.Row{"order": r["order_id"], "node": cfg.Node, "incarnation": cfg.Incarnation, "run": key, "runtime_id": j.ID})
	if err != nil {
		return err
	}
	j, err = s.Execute(j.ID)
	if err != nil {
		return err
	}
	if err = publish(j, ""); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(j)
}

// Persist only the environment needed to locate the already authenticated
// harness. Database/service tokens must not enter the execution journal.
func executionEnvironment() []string {
	out := []string{}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "PATH", "TMPDIR", "CODEX_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+value)
		}
	}
	return out
}
