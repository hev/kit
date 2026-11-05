package dispatch

import (
	"context"
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
	if q.Action != "list" && !ValidAddress(q.Name) {
		return fmt.Errorf("invalid run name")
	}
	dir := OrchestratorDir(s.Name)
	var owner Host
	scopeOnly := readJSON(filepath.Join(dir, "scope-owner.json"), &owner) == nil
	if scopeOnly {
		// An owner's check never lands on another registration. A source
		// start's target probe (ReserveClass) is not an owner check: it is
		// checked with the owner below like any target admission.
		if q.AdmissionOnly && !q.ReserveClass {
			return fmt.Errorf("scope-only owner cannot be another registration")
		}
		check := q
		check.AdmissionOnly = true
		// Class slots are reserved here, on the target, never on the owner.
		check.ReserveClass = false
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if _, e := routeRPC(ctx, owner, check); e != nil {
			return fmt.Errorf("owner admission: %w", e)
		}
	}
	if !scopeOnly && (exists(filepath.Join(dir, "stop")) || exists(filepath.Join(dir, "pause"))) {
		return fmt.Errorf("orchestrator is stopped or paused")
	}
	st, err := orchestratorState(s.Name)
	if err != nil {
		return err
	}
	if !scopeOnly && (st.State == "handed-off" || st.State == "complete") {
		return fmt.Errorf("orchestrator is terminal")
	}
	if !scopeOnly {
		if e := admitSourceOrder(s, q); e != nil {
			return e
		}
	}
	// Lifecycle probes need no run metadata on the owner; targets enforce their
	// local scope. A start probe carries its order, so it gets full admission:
	// a definitive authority or scope rejection must never look uncertain.
	if q.AdmissionOnly && q.Action != "start" {
		return nil
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
	parents, e := orchestratorParents(s.Name)
	if e != nil {
		return e
	}
	for _, parent := range parents {
		pq := q
		pq.Orchestrator = parent.Name
		if e := reserveOrchestratorBudget(pq, live); e != nil {
			return e
		}
	}
	return reserveOrchestratorCounts(s, count, "budget.json")
}

// A model may only create/control explicitly declared children. Lifecycle CLI
// cannot become an escape hatch around dispatch authority or nesting admission.
func admitOrchestratorChild(parent, child string, candidate ...OrchestratorSpec) error {
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
	childSpec, err := LoadOrchestrator(child)
	if len(candidate) > 0 {
		childSpec = candidate[0]
		err = nil
	}
	if err != nil {
		return err
	}
	return orchestratorSubset(s, childSpec)
}

// Owner reservations cover known scoped hosts and the target; target reservations also
// protect direct RPC admission. The owner lock is distinct from dispatch's root
// lock and stays held through launch acknowledgement, closing cross-host races.
func reserveOrchestratorOwner(q Request, target Host) (func(), error) {
	noop := func() {}
	if q.Action != "start" && q.Action != "rework" && q.Action != "retry" && q.Action != "say" {
		return noop, nil
	}
	s, e := LoadOrchestrator(q.Orchestrator)
	if e != nil {
		return nil, e
	}
	parents, e := orchestratorParents(s.Name)
	if e != nil {
		return nil, e
	}
	releases := []func(){}
	for _, parent := range parents {
		pq := q
		pq.Orchestrator = parent.Name
		release, e := reserveOrchestratorOwner(pq, target)
		if e != nil {
			for _, r := range releases {
				r()
			}
			return nil, e
		}
		releases = append(releases, release)
	}
	noop = func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	if s.Budget.ConcurrentRuns == 0 && s.Budget.RunsPerDay == 0 {
		return noop, nil
	}
	release, e := lockDirectory(OrchestratorDir(s.Name))
	if e != nil {
		noop()
		return nil, e
	}
	ownRelease := release
	release = func() { ownRelease(); noop() }
	fail := func(e error) (func(), error) { release(); return nil, e }
	cfg, e := Load(ConfigPath(""))
	if e != nil {
		return fail(e)
	}
	cfg.Hosts = orchestratorBudgetHosts(s, cfg.Hosts, target)
	inv := Inventory(cfg, "")
	if e = reserveOrchestratorInventoryCounts(s, inv); e != nil {
		return fail(e)
	}
	return release, nil
}

func reserveOrchestratorCounts(s OrchestratorSpec, count int, ledger string) error {
	if s.Budget.ConcurrentRuns > 0 && count >= s.Budget.ConcurrentRuns {
		orchestratorBudgetNotice(s, "concurrent_runs")
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
		orchestratorBudgetNotice(s, "runs_per_day")
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

// Subsets are deliberately conservative: selectors must be identical; explicit
// names and repository labels can be narrowed, never broadened.
func orchestratorSubset(parent, child OrchestratorSpec) error {
	contains := func(xs []string, x string) bool {
		for _, v := range xs {
			if v == x {
				return true
			}
		}
		return false
	}
	bad := func() error {
		return fmt.Errorf("nested orchestrator %s exceeds parent %s scope, authority or budget", child.Name, parent.Name)
	}
	for _, a := range child.Authority {
		if !contains(parent.Authority, a) {
			return bad()
		}
	}
	if len(parent.Manages.Repos) > 0 {
		if len(child.Manages.Repos) == 0 {
			return bad()
		}
		for _, r := range child.Manages.Repos {
			if !contains(parent.Manages.Repos, r) {
				return bad()
			}
		}
	}
	if parent.Manages.Runs.Selector != "" {
		if child.Manages.Runs.Selector != parent.Manages.Runs.Selector || (parent.Manages.Runs.Selector == "mine" && child.Reply != parent.Reply) {
			return bad()
		}
	} else {
		if child.Manages.Runs.Selector != "" {
			return bad()
		}
		for _, n := range child.Manages.Runs.Names {
			if !contains(parent.Manages.Runs.Names, n) {
				return bad()
			}
		}
	}
	for _, pair := range [][2]int{{parent.Budget.ConcurrentRuns, child.Budget.ConcurrentRuns}, {parent.Budget.RunsPerDay, child.Budget.RunsPerDay}} {
		if pair[0] > 0 && (pair[1] == 0 || pair[1] > pair[0]) {
			return bad()
		}
	}
	if parent.Budget.USDPerDay > 0 && (child.Budget.USDPerDay == 0 || child.Budget.USDPerDay > parent.Budget.USDPerDay) {
		return bad()
	}
	return nil
}

func orchestratorParents(child string) ([]OrchestratorSpec, error) {
	entries, e := os.ReadDir(OrchestratorConfigDir())
	if e != nil {
		return nil, e
	}
	var out []OrchestratorSpec
	for _, f := range entries {
		if !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		s, e := LoadOrchestrator(strings.TrimSuffix(f.Name(), ".md"))
		if e != nil {
			return nil, e
		}
		for _, n := range s.Manages.Runs.Names {
			if strings.TrimPrefix(n, "orch-") == child {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

// Budget snapshots need the selected target and hosts with known scoped work,
// not every unrelated endpoint in the user's configuration.
func orchestratorBudgetHosts(s OrchestratorSpec, hosts map[string]Host, target Host) map[string]Host {
	out := map[string]Host{}
	add := func(h Host) {
		for n, v := range hosts {
			if v.Local == h.Local && v.SSH == h.SSH && v.Namespace == h.Namespace {
				out[n] = v
			}
		}
	}
	add(target)
	entries, _ := os.ReadDir(Root())
	for _, f := range entries {
		var m Meta
		if readJSON(filepath.Join(Root(), f.Name(), "meta.json"), &m) == nil && s.Includes(m.Order) {
			if m.Forward != nil {
				add(*m.Forward)
			} else if h, ok := hosts[m.Host]; ok {
				add(h)
			}
		}
	}
	st, _ := orchestratorState(s.Name)
	for i := 1; i <= st.Rounds; i++ {
		var packet OrchestratorPacket
		if readJSON(filepath.Join(OrchestratorDir(s.Name), "rounds", fmt.Sprintf("%03d", i), "packet.json"), &packet) == nil {
			for _, ev := range packet.Evidence {
				if s.Includes(ev.Row.Order) {
					if h, ok := hosts[ev.Host]; ok {
						out[ev.Host] = h
					}
				}
			}
		}
	}
	return out
}
