package secrets

import (
	"bufio"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

type SummaryRow struct {
	Caller    string `json:"caller"`
	Provider  string `json:"provider"`
	Requests  int    `json:"requests"`
	CacheHits int    `json:"cache_hits"`
	Throttles int    `json:"throttles"`
}

// Summary returns counts and caller labels only. Requests come from the
// durable reservation ledger, including failed attempts and meter checks;
// free events come from both generations of the read log.
func Summary(now time.Time) ([]SummaryRow, error) {
	rows := map[string]*SummaryRow{}
	row := func(c, p string) *SummaryRow {
		k := c + "\x00" + p
		if rows[k] == nil {
			rows[k] = &SummaryRow{Caller: c, Provider: p}
		}
		return rows[k]
	}
	if err := withLedger(now, func(l *ledger) error {
		for _, s := range l.Spends {
			row(s.Caller, s.Provider).Requests += s.Requests
		}
		return nil
	}); err != nil {
		return nil, err
	}
	for _, path := range []string{ReadLogPath() + ".1", ReadLogPath()} {
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) == 0 {
				continue
			}
			at, err := time.Parse(time.RFC3339, fields[0])
			if err != nil || now.Sub(at) > 24*time.Hour {
				continue
			}
			v := map[string]string{}
			for _, s := range fields[1:] {
				k, x, _ := strings.Cut(s, "=")
				v[k] = x
			}
			p := v["provider"]
			if p == "" {
				p = "op"
			}
			r := row(v["caller"], p)
			switch v["kind"] {
			case "cache":
				r.CacheHits++
			case "throttle":
				r.Throttles++
			}
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	out := []SummaryRow{}
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Caller == out[j].Caller {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Caller < out[j].Caller
	})
	return out, nil
}
