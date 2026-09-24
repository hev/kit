// Package cost prices factory's content-free accounting export. It never reads
// identities or calls an issue tracker. Subscription amounts are allocations.
package cost

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
)

type Usage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cache_read_tokens"`
	Creation   int64 `json:"cache_creation_tokens"`
	Output     int64 `json:"output_tokens"`
	Creation1h int64 `json:"cache_creation_1h_tokens"`
	Reasoning  int64 `json:"reasoning_tokens"`
}

func (u Usage) Tokens() int64 { return u.Input + u.Cached + u.Creation + u.Creation1h + u.Output }

type Request struct {
	Model string `json:"model"`
	TS    int64  `json:"ts"`
	Usage Usage  `json:"usage"`
}

type Row struct {
	Requests        []Request        `json:"requests,omitempty"`
	ID              string           `json:"session_id"`
	Harness         string           `json:"harness"`
	Model           string           `json:"model"`
	CWD             string           `json:"cwd"`
	Instance        string           `json:"instance"`
	Role            string           `json:"role"`
	Issue           string           `json:"issue"`
	IssueURL        string           `json:"issue_url"`
	Repo            string           `json:"repo"`
	Plan            string           `json:"plan"`
	Step            string           `json:"step"`
	Start           int64            `json:"start"`
	End             int64            `json:"end"`
	Usage           map[string]Usage `json:"usage_by_model"`
	Tokens          int64            `json:"total_tokens"`
	API             *float64         `json:"api_usd"`
	Sub             *float64         `json:"sub_usd"`
	Subscription    string           `json:"subscription"`
	Errors          []string         `json:"attribution_errors"`
	AccountingError string           `json:"accounting_error"`
	Limits          json.RawMessage  `json:"rate_limits"`
}
type Rate struct {
	Source      string   `toml:"source"`
	Creation1h  *float64 `toml:"creation_1h"`
	LongContext int64    `toml:"long_context"`
	LongInput   float64  `toml:"long_input_multiplier"`
	LongOutput  float64  `toml:"long_output_multiplier"`
	Model       string   `toml:"model"`
	Effective   string   `toml:"effective"`
	Input       float64  `toml:"input"`
	Cached      float64  `toml:"cached"`
	Creation    float64  `toml:"creation"`
	Output      float64  `toml:"output"`
}
type Plan struct {
	Tier        string   `toml:"tier" json:"tier"`
	ResetMinute int      `toml:"reset_minute" json:"reset_minute"`
	ResetSecond int      `toml:"reset_second" json:"reset_second"`
	ResetHour   int      `toml:"reset_hour" json:"reset_hour"`
	Name        string   `toml:"name" json:"name"`
	Monthly     *float64 `toml:"monthly_usd" json:"monthly_usd"`
	Reset       int      `toml:"reset_day" json:"reset_day"` // Sunday=0, UTC
	Harness     string   `toml:"harness" json:"harness"`
	Models      []string `toml:"models" json:"models"`
	Basis       int64    `toml:"weekly_token_basis" json:"weekly_token_basis"`
}
type Config struct {
	Rates []Rate `toml:"rates"`
	Plans []Plan `toml:"plans"`
}

func LoadConfig(prices, subscriptions string) (Config, error) {
	var c Config
	if _, err := toml.DecodeFile(prices, &c); err != nil {
		return c, err
	}
	if _, err := toml.DecodeFile(subscriptions, &c); err != nil {
		return c, err
	}
	seen := map[string]bool{}
	for _, r := range c.Rates {
		_, err := time.Parse("2006-01-02", r.Effective)
		key := r.Model + "/" + r.Effective
		if err != nil || r.Model == "" || seen[key] || r.Input < 0 || r.Cached < 0 || r.Creation < 0 || r.Output < 0 || invalidNumber(r.Input, r.Cached, r.Creation, r.Output) {
			return c, fmt.Errorf("invalid or duplicate rate %q", key)
		}
		if (r.Creation1h != nil && (*r.Creation1h < 0 || invalidNumber(*r.Creation1h))) || r.LongContext < 0 || (r.LongContext > 0 && (r.LongInput <= 0 || r.LongOutput <= 0 || invalidNumber(r.LongInput, r.LongOutput))) {
			return c, fmt.Errorf("invalid extended rate %q", key)
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for _, p := range c.Plans {
		if p.Name == "" || seen[p.Name] || (p.Monthly != nil && (*p.Monthly < 0 || invalidNumber(*p.Monthly))) || p.Reset < 0 || p.Reset > 6 || p.Harness == "" || p.Basis < 0 || p.ResetHour < 0 || p.ResetHour > 23 || p.ResetMinute < 0 || p.ResetMinute > 59 || p.ResetSecond < 0 || p.ResetSecond > 59 {
			return c, fmt.Errorf("invalid or duplicate subscription %q", p.Name)
		}
		seen[p.Name] = true
	}
	return c, nil
}
func ReadRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byID := map[string]Row{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 4<<20)
	for s.Scan() {
		var r Row
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			return nil, err
		}
		if r.ID == "" {
			return nil, fmt.Errorf("missing session ID")
		}
		byID[r.ID] = r
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(byID))
	for _, r := range byID {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}
func week(ts int64, reset int) int64 {
	t := time.UnixMilli(ts).UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -(int(day.Weekday())-reset+7)%7).UnixMilli()
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (c Config) Price(rows []Row) error {
	totals := map[string]int64{}
	keys := make([]string, len(rows))
	for i := range rows {
		r := &rows[i]
		r.API = nil
		r.Sub = nil
		r.Subscription = ""
		r.Tokens = 0
		api := 0.0
		known := len(r.Usage) > 0 && r.AccountingError == ""
		requests := r.Requests
		if len(requests) == 0 {
			for model, u := range r.Usage {
				requests = append(requests, Request{Model: model, TS: r.Start, Usage: u})
			}
		}
		for _, request := range requests {
			u := request.Usage
			if u.Input < 0 || u.Cached < 0 || u.Creation < 0 || u.Creation1h < 0 || u.Output < 0 {
				return fmt.Errorf("negative usage for %s", r.ID)
			}
			r.Tokens += u.Tokens()
			if u.Tokens() == 0 {
				continue
			}
			var best *Rate
			date := time.UnixMilli(request.TS).UTC().Format("2006-01-02")
			for j := range c.Rates {
				rate := &c.Rates[j]
				if rate.Model == request.Model && rate.Effective <= date && (best == nil || rate.Effective > best.Effective) {
					best = rate
				}
			}
			if best == nil {
				known = false
				continue
			}
			if u.Creation1h > 0 && best.Creation1h == nil {
				known = false
				continue
			}
			inputMultiplier, outputMultiplier := 1.0, 1.0
			if best.LongContext > 0 && u.Input+u.Cached+u.Creation+u.Creation1h > best.LongContext {
				if len(r.Requests) == 0 {
					known = false
					continue
				} // Aggregate context is not request context.
				inputMultiplier, outputMultiplier = best.LongInput, best.LongOutput
			}
			creation1h := 0.0
			if best.Creation1h != nil {
				creation1h = float64(u.Creation1h) * *best.Creation1h
			}
			api += ((float64(u.Input)*best.Input+float64(u.Cached)*best.Cached+float64(u.Creation)*best.Creation+creation1h)*inputMultiplier + float64(u.Output)*best.Output*outputMultiplier) / 1e6
		}
		if known {
			r.API = &api
		}
		for _, p := range c.Plans {
			if p.Harness != r.Harness {
				continue
			}
			matches := true
			for m := range r.Usage {
				if len(p.Models) > 0 && !contains(p.Models, m) {
					matches = false
				}
			}
			if !matches {
				continue
			}
			if r.Subscription != "" {
				return fmt.Errorf("session %s matches multiple subscriptions", r.ID)
			}
			r.Subscription = p.Name
			keys[i] = fmt.Sprintf("%s/%d", p.Name, planWeek(r.Start, p))
			totals[keys[i]] += r.Tokens
		}
	}
	for i := range rows {
		r := &rows[i]
		for _, p := range c.Plans {
			if r.Subscription == p.Name && totals[keys[i]] > 0 && p.Monthly != nil {
				v := *p.Monthly * 12 / 365.2425 * 7 * float64(r.Tokens) / float64(totals[keys[i]])
				r.Sub = &v
			}
		}
	}
	return nil
}

func invalidNumber(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return true
		}
	}
	return false
}

func planWeek(ts int64, p Plan) int64 {
	offset := int64(p.ResetHour)*3600000 + int64(p.ResetMinute)*60000 + int64(p.ResetSecond)*1000
	return week(ts-offset, p.Reset) + offset
}

// InCurrentWeek uses the matched plan's actual UTC reset, not the view window.
func (c Config) InCurrentWeek(r Row, now time.Time) bool {
	for _, p := range c.Plans {
		if r.Subscription == p.Name {
			return planWeek(r.Start, p) == planWeek(now.UnixMilli(), p)
		}
	}
	return false
}
