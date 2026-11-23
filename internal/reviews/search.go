package reviews

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/hev/kit/internal/grep"
)

type Filter struct {
	Limit                                      int
	Repo, Order, Head, Verdict, Path, Severity string
	Open                                       bool
}

func (s *Store) List(ctx context.Context, f Filter) ([]Review, error) {
	return s.scan(ctx, f, nil)
}

func (s *Store) Search(ctx context.Context, query string, f Filter) ([]Review, error) {
	if len(query) > 4096 {
		return nil, errors.New("search query exceeds size limit")
	}
	q, err := grep.Compile([]string{query})
	if err != nil || len(q.Needles()) == 0 {
		return nil, errors.New("search requires a lexical query")
	}
	return s.scan(ctx, f, q)
}

func (s *Store) scan(ctx context.Context, f Filter, q *grep.Query) ([]Review, error) {
	if f.Verdict != "" && !validVerdict(f.Verdict) {
		return nil, errors.New("verdict must be pass, rework, or comment")
	}
	if f.Severity != "" && !validSeverity(f.Severity) {
		return nil, errors.New("severity must be blocker, major, minor, or nit")
	}
	if f.Head != "" && !ValidSHA(f.Head) {
		return nil, errors.New("filter requires a full head SHA")
	}
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 200 {
		return nil, errors.New("review result limit must be between 1 and 200")
	}
	s.scrub(&f.Repo, &f.Order)
	result := []Review{}
	dir, err := os.Open(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, errors.New("could not open review store")
	}
	defer dir.Close()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := dir.ReadDir(128)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("could not scan review store")
		}
		for _, entry := range entries {
			if !entry.IsDir() || !validID("rev_", entry.Name()) {
				continue
			}
			r, err := s.Get(entry.Name())
			if errors.Is(err, fs.ErrNotExist) {
				continue // A concurrent create has not published its review yet.
			}
			if err != nil {
				return nil, err
			}
			if !f.matches(&r, q) {
				continue
			}
			at := sort.Search(len(result), func(i int) bool {
				return r.CreatedAt.After(result[i].CreatedAt) ||
					(r.CreatedAt.Equal(result[i].CreatedAt) && r.ID > result[i].ID)
			})
			if at < f.Limit {
				if len(result) < f.Limit {
					result = append(result, Review{})
				}
				copy(result[at+1:], result[at:])
				result[at] = r
			}
		}
	}
	return result, nil
}

func (f Filter) matches(r *Review, q *grep.Query) bool {
	if (f.Repo != "" && r.Repo != f.Repo) || (f.Order != "" && r.Order != f.Order) ||
		(f.Head != "" && !strings.EqualFold(r.HeadSHA, f.Head)) || (f.Verdict != "" && r.Verdict != f.Verdict) {
		return false
	}
	filtered := f.Open || f.Path != "" || f.Severity != ""
	var text strings.Builder
	if q != nil {
		text.WriteString(r.Summary)
	}
	var findings []Finding
	for _, finding := range r.Findings {
		if (f.Open && finding.Status != "open") || !strings.Contains(finding.File, f.Path) ||
			(f.Severity != "" && finding.Severity != f.Severity) {
			continue
		}
		findings = append(findings, finding)
		if q != nil {
			text.WriteString("\n" + finding.File + "\n" + finding.Body)
		}
	}
	if filtered && len(findings) == 0 {
		return false
	}
	if q != nil {
		if _, ok := q.Score(text.String()); !ok {
			return false
		}
		// Search results include only findings eligible under the filters.
		if filtered {
			r.Findings = findings
		}
	}
	return true
}
