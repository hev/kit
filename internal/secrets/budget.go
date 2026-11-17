package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// Policy is read exclusively from this host's secrets-policy.json. It is
// never accepted from an endpoint or dispatch request. Omitted fields select defaults.
type Policy struct {
	AccountDaily  map[string]int `json:"account_daily,omitempty"`
	AccountHourly map[string]int `json:"account_hourly,omitempty"`
	MeterHost     bool           `json:"meter_host"`
	MeterInterval string         `json:"meter_interval,omitempty"`
	Daily         int            `json:"daily"`
	Hourly        int            `json:"hourly"`
	CallerMinute  int            `json:"caller_minute"`
	DisableStale  bool           `json:"disable_stale"`
	Notify        []string       `json:"notify"`
	Thresholds    []int          `json:"thresholds"`
	// Override is a unique operator-chosen nonce granting one extra operation.
	Override string `json:"override_once"`
}
type Spend struct {
	At       time.Time `json:"at"`
	Caller   string    `json:"caller"`
	Provider string    `json:"provider"`
	Requests int       `json:"requests"`
}
type ledger struct {
	Spends     []Spend         `json:"spends"`
	Throttles  int             `json:"throttles"`
	LastCaller string          `json:"last_caller,omitempty"`
	Override   string          `json:"override,omitempty"`
	Warnings   map[string]bool `json:"warnings"`
}
type BudgetStatus struct {
	Daily        int    `json:"daily"`
	Hourly       int    `json:"hourly"`
	UsedDay      int    `json:"used_day"`
	UsedHour     int    `json:"used_hour"`
	CallerMinute int    `json:"caller_minute"`
	Throttles    int    `json:"throttles"`
	LastCaller   string `json:"last_caller,omitempty"`
	Error        string `json:"error,omitempty"`
}

func policyDir() string {
	if p := os.Getenv("HEV_SECRETS_POLICY"); p != "" {
		return filepath.Dir(p)
	}
	if p := ReadLogPath(); p != "" {
		return filepath.Dir(p)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hev", "secrets")
}
func policyPath() string {
	if p := os.Getenv("HEV_SECRETS_POLICY"); p != "" {
		return p
	}
	return filepath.Join(policyDir(), "secrets-policy.json")
}
func hostPolicy() (Policy, error) {
	p := Policy{Daily: 200, Hourly: 40, CallerMinute: 5, Thresholds: []int{50, 80}}
	b, err := os.ReadFile(policyPath())
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, errors.New("secrets policy unreadable")
	}
	if json.Unmarshal(b, &p) != nil {
		return p, errors.New("invalid secrets policy")
	}
	if p.Daily <= 0 || p.Hourly <= 0 || p.CallerMinute <= 0 {
		return p, errors.New("secrets budgets must be positive")
	}
	for _, limits := range []map[string]int{p.AccountDaily, p.AccountHourly} {
		for provider, n := range limits {
			if (provider != "op" && provider != "azkv") || n <= 0 {
				return p, errors.New("invalid account budget")
			}
		}
	}
	if p.MeterInterval != "" {
		d, err := time.ParseDuration(p.MeterInterval)
		if err != nil || d <= 0 {
			return p, errors.New("invalid meter interval")
		}
	}
	for _, n := range p.Thresholds {
		if n <= 0 || n > 100 {
			return p, errors.New("invalid secrets warning threshold")
		}
	}
	return p, nil
}

// withLedger locks across processes, reserves before a request, and fails
// closed on corruption or write failure. No secret, URI or token is stored.
func withLedger(now time.Time, f func(*ledger) error) error {
	dir := policyDir()
	if err := platformops.MkdirAll(dir, 0700); err != nil {
		return err
	}
	unlock, err := platformops.LockPath(filepath.Join(dir, "budget.lock"), false)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(dir, "budget.json")
	l := ledger{Warnings: map[string]bool{}}
	b, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(b, &l) != nil {
			return errors.New("secrets budget ledger corrupt; operator repair required")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if l.Warnings == nil {
		l.Warnings = map[string]bool{}
	}
	keep := l.Spends[:0]
	for _, s := range l.Spends {
		if now.Sub(s.At) < 24*time.Hour {
			keep = append(keep, s)
		}
	}
	l.Spends = keep
	result := f(&l)
	b, err = json.Marshal(l)
	if err != nil {
		return err
	}
	if err := platformops.WriteFileAtomic(path, b, 0600); err != nil {
		return errors.New("cannot persist secrets budget; request refused")
	}
	return result
}
func budgetView(p Policy, l *ledger, now time.Time) BudgetStatus {
	s := BudgetStatus{Daily: p.Daily, Hourly: p.Hourly, CallerMinute: p.CallerMinute, Throttles: l.Throttles, LastCaller: l.LastCaller}
	for _, v := range l.Spends {
		s.UsedDay += v.Requests
		if now.Sub(v.At) < time.Hour {
			s.UsedHour += v.Requests
		}
	}
	return s
}

// IsBudgetError works through an older endpoint which preserves only Error.
func IsBudgetError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "secrets budget:")
}
func reserve(now time.Time, caller, provider string, n int) error {
	return reserveWith(now, caller, provider, n, nil)
}
func reserveWith(now time.Time, caller, provider string, n int, c *Cache) error {
	p, err := hostPolicy()
	if err != nil {
		return fmt.Errorf("secrets budget: %v", err)
	}
	caller = sanitizeField(caller, 96)
	err = withLedger(now, func(l *ledger) error {
		s := budgetView(p, l, now)
		used := 0
		for _, v := range l.Spends {
			if v.Caller == caller && now.Sub(v.At) < time.Minute {
				used += v.Requests
			}
		}
		reason := ""
		if used+n > p.CallerMinute {
			reason = "caller throttled"
		}
		if s.UsedHour+n > p.Hourly {
			reason = "host hourly allowance exhausted"
		}
		if s.UsedDay+n > p.Daily {
			reason = "host daily allowance exhausted"
		}
		if c != nil {
			if account := c.accountReason(l, p, now, provider, n); account != "" {
				l.Throttles++
				l.LastCaller = caller
				return fmt.Errorf("secrets budget: %s", account)
			}
		}
		if reason != "" && !(p.Override != "" && p.Override != l.Override) {
			l.Throttles++
			l.LastCaller = caller
			return fmt.Errorf("secrets budget: %s; wait for the rolling window or raise host policy", reason)
		}
		if reason != "" {
			l.Override = p.Override
		}
		l.Spends = append(l.Spends, Spend{now, caller, provider, n})
		if c != nil && c.clusterUsage() != nil {
			_ = c.publishLedger(l, p, now)
		}
		return nil
	})
	if err != nil {
		if !IsBudgetError(err) {
			err = fmt.Errorf("secrets budget: %v", err)
		}
		LogReadProvider(now, provider, caller, "throttle", nil, "throttled")
	}
	return err
}
func budgetStatus(now time.Time) BudgetStatus {
	p, err := hostPolicy()
	if err != nil {
		return BudgetStatus{Error: err.Error()}
	}
	var s BudgetStatus
	err = withLedger(now, func(l *ledger) error { s = budgetView(p, l, now); return nil })
	if err != nil {
		s.Error = err.Error()
	}
	return s
}

// warnings records crossings before invoking the configured argv (no shell).
// Each threshold is rearmed once usage falls below it. Reservations and this
// state survive restart. Notification failure is visible in status.
func (c *Cache) warnings(ctx context.Context, s Status) {
	p, err := hostPolicy()
	if err != nil || len(p.Notify) == 0 {
		return
	}
	events := []string{}
	err = withLedger(c.now(), func(l *ledger) error {
		b := budgetView(p, l, c.now())
		levels := map[string]int{"host-day": 100 * b.UsedDay / p.Daily, "host-hour": 100 * b.UsedHour / p.Hourly}
		if p.MeterHost {
			for _, a := range s.Accounts {
				if a.Daily > 0 {
					levels["account-"+a.Provider+"-day"] = int(100 * a.UsedDay / uint64(a.Daily))
				}
				if a.Hourly > 0 {
					levels["account-"+a.Provider+"-hour"] = int(100 * a.UsedHour / uint64(a.Hourly))
				}
			}
		}
		for _, q := range s.Quota {
			if c.clusterUsage() != nil && !p.MeterHost {
				continue
			}
			if q.Limit != nil && *q.Limit > 0 {
				used := q.Used
				if used == nil && q.Remaining != nil {
					v := *q.Limit - *q.Remaining
					used = &v
				}
				if used != nil {
					levels[q.Type+"-"+q.Action] = int(100 * *used / *q.Limit)
				}
			}
		}
		for k, v := range levels {
			for _, t := range p.Thresholds {
				key := fmt.Sprintf("%s-%d", k, t)
				if v >= t {
					if !l.Warnings[key] {
						events = append(events, key)
					}
					l.Warnings[key] = true
				} else {
					delete(l.Warnings, key)
				}
			}
		}
		return nil
	})
	if err != nil {
		return
	}
	sort.Strings(events)
	for _, e := range events {
		nc, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(nc, p.Notify[0], append(p.Notify[1:], e)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		err := cmd.Run()
		cancel()
		if err != nil {
			c.mu.Lock()
			c.notifyError = "secrets notify command failed"
			c.mu.Unlock()
		}
	}
}

// ReserveDirect covers legacy kit call sites that own an op subprocess.
// External op calls do not invoke it and escape kit's guard.
func ReserveDirect(now time.Time, caller string) error { return reserve(now, caller, "op", 3) }
