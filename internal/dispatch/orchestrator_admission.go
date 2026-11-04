package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// admitOrchestrator is enforced at the target, before routing or mutation. The
// sender forwards identity, never a sender-selected authority or scope document.
func admitOrchestrator(q Request) error {
	if q.Orchestrator == "" {
		return nil
	}
	s, e := LoadOrchestrator(q.Orchestrator)
	if e != nil {
		return fmt.Errorf("orchestrator admission: %w", e)
	}
	if q.Action != "list" && !ValidName(q.Name) {
		return fmt.Errorf("invalid run name")
	}
	dir := OrchestratorDir(s.Name)
	if exists(filepath.Join(dir, "stop")) || exists(filepath.Join(dir, "pause")) {
		return fmt.Errorf("orchestrator is stopped or paused")
	}
	st, err := orchestratorState(s.Name)
	if err != nil {
		return err
	}
	if st.State == "handed-off" || st.State == "complete" {
		return fmt.Errorf("orchestrator is terminal")
	}
	verb := q.Action
	switch verb {
	case "list":
		return nil
	case "start":
		verb = "dispatch"
		if q.Meta.Parent != "" {
			verb = "validate"
		}
	case "validation-reserve", "validation-result":
		verb = "validate"
	case "peer-send":
		verb = "say"
	}
	read := verb == "show" || verb == "logs"
	allowed := read
	for _, a := range s.Authority {
		if a == verb {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("orchestrator %s lacks authority %s", s.Name, verb)
	}
	var o Order
	if q.Action == "start" {
		o, e = Parse(q.Order)
	} else {
		var m Meta
		e = readJSON(filepath.Join(Root(), q.Name, "meta.json"), &m)
		o = m.Order
	}
	if e != nil {
		return e
	}
	expectedName := q.Name
	if q.Action == "start" && q.Meta.Parent != "" {
		expectedName = q.Meta.Parent
		suffix, ok := strings.CutPrefix(q.Name, expectedName+"-v")
		n, err := strconv.Atoi(suffix)
		if !ok || err != nil || n < 1 || q.Meta.Repo != o.Repo || q.Meta.Validation != "none" {
			return fmt.Errorf("invalid scoped validator")
		}
		cfg, err := Load(ConfigPath(""))
		if err != nil {
			return err
		}
		if q.Meta.Harness != cfg.Validation.Harness || q.Meta.Model != cfg.Validation.Model {
			return fmt.Errorf("validator must use configured harness and model")
		}
	}
	if o.Name != expectedName || !s.Includes(o) {
		return fmt.Errorf("orchestrator %s: run %s outside manages scope", s.Name, q.Name)
	}
	return nil
}

// reserveOrchestratorBudget is called under dispatch's admission lock. Count
// live runs in scope and persist a daily attempt receipt before launching. Failed
// launches still consume an attempt, so retries cannot evade the daily cap.
func reserveOrchestratorBudget(q Request, live map[string]bool) error {
	if q.Orchestrator == "" {
		return nil
	}
	s, e := LoadOrchestrator(q.Orchestrator)
	if e != nil {
		return e
	}
	if exists(filepath.Join(OrchestratorDir(s.Name), "stop")) || exists(filepath.Join(OrchestratorDir(s.Name), "pause")) {
		return fmt.Errorf("orchestrator is stopped or paused")
	}
	count := 0
	for n, active := range live {
		if !active {
			continue
		}
		var m Meta
		if readJSON(filepath.Join(Root(), n, "meta.json"), &m) == nil {
			if m.Parent != "" {
				m.Order.Name = m.Parent
			}
			if s.Includes(m.Order) {
				count++
			}
		}
	}
	return reserveOrchestratorCounts(s, count, "budget.json")
}

// A model may only create/control explicitly declared children. Lifecycle CLI
// cannot become an escape hatch around dispatch authority or nesting admission.
func admitOrchestratorChild(parent, child string) error {
	s, e := LoadOrchestrator(parent)
	if e != nil {
		return e
	}
	if exists(filepath.Join(OrchestratorDir(parent), "pause")) || exists(filepath.Join(OrchestratorDir(parent), "stop")) {
		return fmt.Errorf("parent orchestrator is paused or stopped")
	}
	st, e := orchestratorState(parent)
	if e != nil {
		return e
	}
	if st.State == "handed-off" || st.State == "complete" {
		return fmt.Errorf("parent orchestrator is terminal")
	}
	if parent == child || !s.AllowNested {
		return fmt.Errorf("nested orchestrator requires allow_nested and distinct child")
	}
	allowed := false
	for _, a := range s.Authority {
		if a == "dispatch" {
			allowed = true
		}
	}
	named := false
	for _, n := range s.Manages.Runs.Names {
		if n == child || n == "orch-"+child {
			named = true
		}
	}
	if !allowed || !named {
		return fmt.Errorf("nested orchestrator outside authority or manages scope")
	}
	return nil
}

// Owner reservations cover all configured hosts, while target reservations also
// protect direct RPC admission. The owner lock is distinct from dispatch's root
// lock and stays held through launch acknowledgement, closing cross-host races.
func reserveOrchestratorOwner(q Request) (func(), error) {
	noop := func() {}
	if q.Action != "start" && q.Action != "rework" && q.Action != "retry" && q.Action != "say" {
		return noop, nil
	}
	s, e := LoadOrchestrator(q.Orchestrator)
	if e != nil {
		return nil, e
	}
	if s.Budget.ConcurrentRuns == 0 && s.Budget.RunsPerDay == 0 {
		return noop, nil
	}
	release, e := lockDirectory(OrchestratorDir(s.Name))
	if e != nil {
		return nil, e
	}
	fail := func(e error) (func(), error) { release(); return nil, e }
	cfg, e := Load(ConfigPath(""))
	if e != nil {
		return fail(e)
	}
	inv := Inventory(cfg, "")
	if e = reserveOrchestratorInventoryCounts(s, inv); e != nil {
		return fail(e)
	}
	return release, nil
}

func reserveOrchestratorCounts(s OrchestratorSpec, count int, ledger string) error {
	if s.Budget.ConcurrentRuns > 0 && count >= s.Budget.ConcurrentRuns {
		return fmt.Errorf("orchestrator concurrent_runs budget exhausted")
	}
	var day struct {
		Day  string `json:"day"`
		Runs int    `json:"runs"`
	}
	p := filepath.Join(OrchestratorDir(s.Name), ledger)
	if e := readJSON(p, &day); e != nil && !os.IsNotExist(e) {
		return e
	}
	today := time.Now().UTC().Format("2006-01-02")
	if day.Day != today {
		day.Day = today
		day.Runs = 0
	}
	if s.Budget.RunsPerDay > 0 && day.Runs >= s.Budget.RunsPerDay {
		return fmt.Errorf("orchestrator runs_per_day budget exhausted")
	}
	day.Runs++
	return WriteJSON(p, day)
}

func reserveOrchestratorInventoryCounts(s OrchestratorSpec, inv map[string]Response) error {
	// Name and repo filters apply to the same inventory used by round gathering.
	rows, e := SelectInventoryEvidence(inv, func(row Row) bool {
		o := row.Order
		if row.Parent != "" {
			o.Name = row.Parent
		}
		return s.Includes(o)
	})
	if e != nil {
		return e
	}
	count := 0
	for _, r := range rows {
		if r.Row.State == "running" || r.Row.State == "launch-unconfirmed" {
			count++
		}
	}
	return reserveOrchestratorCounts(s, count, "budget-owner.json")
}
