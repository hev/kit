package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/version"
)

type evalItem struct {
	ID         int     `json:"id"`
	Type       string  `json:"type"`
	Provenance string  `json:"provenance"`
	Question   string  `json:"question"`
	Gold       *string `json:"gold"`
	Sources    []int   `json:"sources"`
	Superseded []int   `json:"superseded,omitempty"`
	Judgment   string  `json:"judgment"`
}
type evalFixture struct {
	Version json.RawMessage `json:"version"`
	Items   []evalItem      `json:"items"`
	Triage  json.RawMessage `json:"triage,omitempty"`
}
type evalHit struct {
	store.Hit
	Score *float64 `json:"score,omitempty"`
}
type evalRow struct {
	evalItem
	TopK                []int `json:"top_k"`
	Hit                 bool  `json:"hit"`
	Rank                *int  `json:"rank"`
	SupersededSurfaced  bool  `json:"superseded_surfaced"`
	StaleAsCurrent      bool  `json:"stale_as_current"`
	Tokens              int   `json:"tokens"`
	AbstentionRaw       *bool `json:"abstention_raw,omitempty"`
	AbstentionThreshold *bool `json:"abstention_threshold,omitempty"`
}
type evalRate struct {
	Hits  int     `json:"hits"`
	Total int     `json:"total"`
	Rate  float64 `json:"rate"`
}

func (r *evalRate) add(hit bool) {
	r.Total++
	if hit {
		r.Hits++
	}
	r.Rate = float64(r.Hits) / float64(r.Total)
}

type evalAggregates struct {
	Recall        evalRate            `json:"recall_at_k"`
	Abstention    evalRate            `json:"abstention_precision"`
	AbstentionRaw evalRate            `json:"abstention_precision_raw"`
	Correct       evalRate            `json:"correct_and_current"`
	Stale         int                 `json:"stale_as_current"`
	Superseded    int                 `json:"superseded_surfaced"`
	MeanTokens    float64             `json:"mean_tokens"`
	P90Tokens     int                 `json:"p90_tokens"`
	ByType        map[string]evalRate `json:"by_type"`
	ByProvenance  map[string]evalRate `json:"by_provenance"`
}
type evalResult struct {
	BoardURL       string          `json:"board_url"`
	Time           time.Time       `json:"time"`
	KitVersion     string          `json:"kit_version"`
	FixtureVersion json.RawMessage `json:"fixture_version"`
	K              int             `json:"k"`
	Board          string          `json:"board"`
	Threshold      *float64        `json:"threshold"`
	TokenEstimator string          `json:"token_estimator"`
	Items          []evalRow       `json:"items"`
	Aggregates     evalAggregates  `json:"aggregates"`
	Triage         json.RawMessage `json:"triage,omitempty"`
}

// estimateExcerptTokens counts word runs and each punctuation/symbol rune.
func estimateExcerptTokens(s string) int {
	n := 0
	word := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if !word {
				n++
			}
			word = true
		} else {
			word = false
			if !unicode.IsSpace(r) {
				n++
			}
		}
	}
	return n
}
func runEval(args []string) error {
	c := flags("eval")
	k := c.fs.Int("k", 5, "number of search results")
	board := c.fs.String("board", "", "restrict board")
	out := c.fs.String("out", "", "write JSON result")
	only := c.fs.String("only", "", "comma-separated item IDs")
	threshold := c.fs.Float64("threshold", 0, "abstain when every score is below this value")
	pos, err := c.parse(args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *k < 1 {
		return fmt.Errorf("usage: hev board eval FIXTURE.json [--k positive-integer]")
	}
	var th *float64
	c.fs.Visit(func(f *flag.Flag) {
		if f.Name == "threshold" {
			th = threshold
		}
	})
	if th != nil && (math.IsNaN(*th) || math.IsInf(*th, 0)) {
		return fmt.Errorf("threshold must be finite")
	}
	b, err := os.ReadFile(pos[0])
	if err != nil {
		return err
	}
	var fixture evalFixture
	if err = json.Unmarshal(b, &fixture); err != nil {
		return err
	}
	if len(fixture.Version) == 0 || len(fixture.Items) == 0 {
		return fmt.Errorf("fixture requires version and items")
	}
	selected := map[int]bool{}
	if *only != "" {
		for _, s := range strings.Split(*only, ",") {
			id, e := strconv.Atoi(s)
			if e != nil {
				return fmt.Errorf("invalid item ID %q", s)
			}
			selected[id] = true
		}
	}
	seen := map[int]bool{}
	for _, i := range fixture.Items {
		if seen[i.ID] {
			return fmt.Errorf("duplicate item ID %d", i.ID)
		}
		seen[i.ID] = true
		if strings.TrimSpace(i.Question) == "" {
			return fmt.Errorf("item %d has no question", i.ID)
		}
	}
	for id := range selected {
		if !seen[id] {
			return fmt.Errorf("unknown item ID %d", id)
		}
	}
	result := evalResult{BoardURL: baseURL(), Time: time.Now().UTC(), KitVersion: version.String(), FixtureVersion: fixture.Version, K: *k, Board: *board, Threshold: th, TokenEstimator: "word-runs-and-punctuation-v1", Items: []evalRow{}, Triage: fixture.Triage}
	a := evalAggregates{ByType: map[string]evalRate{}, ByProvenance: map[string]evalRate{}}
	tokens := []int{}
	for _, item := range fixture.Items {
		if len(selected) > 0 && !selected[item.ID] {
			continue
		}
		q := url.Values{"q": {item.Question}, "top": {strconv.Itoa(*k)}}
		if *board != "" {
			q.Set("board", *board)
		}
		var hits []evalHit
		if err := call("GET", "/api/search?"+q.Encode(), nil, &hits); err != nil {
			return fmt.Errorf("item %d: %w", item.ID, err)
		}
		if len(hits) > *k {
			hits = hits[:*k]
		}
		row := evalRow{evalItem: item, TopK: []int{}}
		supersededRank := 0
		for j, h := range hits {
			row.TopK = append(row.TopK, int(h.Thread.ID))
			row.Tokens += estimateExcerptTokens(h.Snippet)
			for _, id := range item.Sources {
				if id == int(h.Thread.ID) && row.Rank == nil {
					rank := j + 1
					row.Rank = &rank
				}
			}
			for _, id := range item.Superseded {
				if id == int(h.Thread.ID) && supersededRank == 0 {
					supersededRank = j + 1
				}
			}
		}
		row.Hit = row.Rank != nil
		row.SupersededSurfaced = supersededRank > 0 && (row.Rank == nil || supersededRank < *row.Rank)
		if item.Type == "abstention" {
			raw := len(hits) == 0
			row.AbstentionRaw = &raw
			row.Hit = raw
			if th != nil {
				pass := true
				for _, h := range hits {
					if h.Score == nil {
						return fmt.Errorf("item %d: search API does not expose scores; cannot evaluate --threshold", item.ID)
					}
					if *h.Score >= *th {
						pass = false
					}
				}
				row.AbstentionThreshold = &pass
				row.Hit = pass
			}
			a.Abstention.add(row.Hit)
			a.AbstentionRaw.add(raw)
		} else {
			a.Recall.add(row.Hit)
			r := a.ByType[item.Type]
			r.add(row.Hit)
			a.ByType[item.Type] = r
			r = a.ByProvenance[item.Provenance]
			r.add(row.Hit)
			a.ByProvenance[item.Provenance] = r
		}
		a.Correct.add(row.Hit && item.Judgment == "right")
		row.StaleAsCurrent = row.Hit && (item.Judgment == "stale" || item.Judgment == "wrong")
		if row.StaleAsCurrent {
			a.Stale++
		}
		if row.SupersededSurfaced {
			a.Superseded++
		}
		tokens = append(tokens, row.Tokens)
		a.MeanTokens += float64(row.Tokens)
		result.Items = append(result.Items, row)
	}
	if len(tokens) > 0 {
		a.MeanTokens /= float64(len(tokens))
		sort.Ints(tokens)
		a.P90Tokens = tokens[int(math.Ceil(.9*float64(len(tokens))))-1]
	}
	result.Aggregates = a
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if *out != "" {
		if err = os.WriteFile(*out, append(encoded, '\n'), 0600); err != nil {
			return err
		}
	}
	if *c.json {
		fmt.Println(string(encoded))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tTYPE\tHIT\tRANK\tSUPERSEDED\tSTALE\tTOKENS\tRAW ABSTAIN\tTHRESHOLD ABSTAIN\tTOP K\n")
	for _, r := range result.Items {
		rank := "null"
		if r.Rank != nil {
			rank = strconv.Itoa(*r.Rank)
		}
		boolean := func(v *bool) string {
			if v == nil {
				return "off"
			}
			return strconv.FormatBool(*v)
		}
		fmt.Fprintf(w, "%d\t%s\t%t\t%s\t%t\t%t\t%d\t%s\t%s\t%v\n", r.ID, r.Type, r.Hit, rank, r.SupersededSurfaced, r.StaleAsCurrent, r.Tokens, boolean(r.AbstentionRaw), boolean(r.AbstentionThreshold), r.TopK)
	}
	w.Flush()
	fmt.Printf("\nAggregates (k=%d):\n", *k)
	summary, _ := json.MarshalIndent(a, "", "  ")
	fmt.Println(string(summary))
	return nil
}
