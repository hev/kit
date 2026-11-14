package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/pkg/dashboard"
	"github.com/hev/kit/pkg/kitapi"
)

// kitSignedIn is this machine's kit sign-in, or nil when it has none (or
// the config cannot be read, in which case the caller's usual path reports
// the error).
func kitSignedIn() *daemon.KitConfig {
	k, err := daemon.ReadKitConfig()
	if err != nil || !k.SignedIn() {
		return nil
	}
	return k
}

// kitClient is the kit API client for a signed-in machine.
var kitClient = func(k *daemon.KitConfig) *kitapi.Client { return daemon.KitClient(k) }

// kitWindow maps --since onto the dashboard's search windows, rounding up
// so nothing inside --since is left out.
func kitWindow(since string) (string, error) {
	if since == "" {
		return "all", nil
	}
	d, err := parseSinceDuration(since)
	if err != nil {
		return "", err
	}
	for _, days := range []int{7, 30, 90} {
		if d <= time.Duration(days)*24*time.Hour {
			return fmt.Sprintf("%dd", days), nil
		}
	}
	return "all", nil
}

// runKitSearch is `hev traces search` on a signed-in machine: the kit API
// searches the team's archive and answers one best passage per session.
// Each phrasing is its own request; a session keeps its best score.
func runKitSearch(ctx context.Context, out io.Writer, c *kitapi.Client, phrasings []string, top int, asJSON bool) error {
	if queryOrder != "" || queryPlan != "" || queryWorkdir != "" || queryContext != "" {
		return fmt.Errorf("--order, --plan, --workdir and --context are not available when signed in to kit; filter with --harness and --since")
	}
	window, err := kitWindow(querySince)
	if err != nil {
		return err
	}
	q := url.Values{"window": {window}}
	if queryHarness != "" {
		q.Set("harness", queryHarness)
	}
	if top > 0 {
		q.Set("top", fmt.Sprint(top))
	}
	best := map[string]kitapi.SearchHit{}
	for _, p := range phrasings {
		res, err := c.Search(ctx, p, kitapi.ModeHybrid, q)
		if err != nil {
			return err
		}
		for _, h := range res.Sessions {
			if old, ok := best[h.ID]; !ok || h.Score > old.Score {
				best[h.ID] = h
			}
		}
	}
	hits := make([]kitapi.SearchHit, 0, len(best))
	for _, h := range best {
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Score > hits[j].Score
	})
	if top > 0 && len(hits) > top {
		hits = hits[:top]
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(hits)
	}
	if len(hits) == 0 {
		fmt.Fprintln(out, "nothing found")
		return nil
	}
	for _, h := range hits {
		when := h.HitTS
		if t, err := time.Parse(time.RFC3339, h.HitTS); err == nil {
			when = t.Local().Format("2006-01-02 15:04")
		}
		head := strings.TrimSpace(fmt.Sprintf("%s  %s  %s  %s", when, h.Harness, h.Host, h.Title))
		fmt.Fprintf(out, "\n%s\n  %s\n  session %s  turn %s\n", head, oneParagraph(h.Snippet, 400), h.ID, h.TurnUUID)
	}
	return nil
}

// kitDashboard serves the local dashboard page with its /api reads answered
// by the kit API, so `hev serve` shows the team's archive on a signed-in
// machine. Routes the kit API does not carry answer 404.
func kitDashboard(c *kitapi.Client) http.Handler {
	page := dashboard.NewReaderHandler(emptyReader{}, dashboard.ReaderOptions{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			page.ServeHTTP(w, r)
			return
		}
		p, ok := dashboard.APIPath(r.URL.Path)
		if !ok || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not available from the kit API"}`, http.StatusNotFound)
			return
		}
		raw, err := c.DoRaw(r.Context(), "GET", "/v1/traces"+p, r.URL.Query(), nil)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			code := http.StatusBadGateway
			var e *kitapi.Error
			if errors.As(err, &e) && e.Status < 500 {
				code = e.Status
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_, _ = w.Write(raw)
	})
}

func serveKitDashboard(out io.Writer, ln net.Listener, c *kitapi.Client, team string) error {
	fmt.Fprintf(out, "hev read side (kit team %s): http://%s\n", team, ln.Addr())
	return http.Serve(ln, kitDashboard(c))
}

// emptyReader backs the page handler; every /api read goes to the kit API.
type emptyReader struct{}

func (emptyReader) ListSessionRows(int, any) ([]trace.SessionRow, error) { return nil, nil }
func (emptyReader) ListBlockRows(string) ([]trace.BlockRow, error)       { return nil, nil }
