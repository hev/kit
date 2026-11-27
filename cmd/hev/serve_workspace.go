package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/evidence"
	"github.com/hev/kit/internal/grep"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/license"
	"github.com/hev/kit/internal/peers"
	"github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/workspace"
	"github.com/hev/kit/pkg/kitapi"
)

func serveLocalWorkspace(ctx context.Context, out io.Writer, server *serve.Server, key string) *serve.Server {
	status, configured := license.Inspect(time.Now())
	if configured && !status.Entitled() && status.Reason == "status unavailable" {
		status = license.Resolve(ctx)
	}
	cfg := serve.Workspace{Tier: serve.OSS, Secrets: []string{key, layerenv.Get("HEVLAYER_API_KEY")}}
	if lic, err := license.Configuration(); err == nil {
		cfg.Secrets = append(cfg.Secrets, lic.Key, lic.KeyRef)
	}
	if !status.Entitled() {
		return server.WithWorkspace(cfg)
	}

	cfg.Tier = serve.Pro
	cfg.Settings = []serve.WorkspaceRow{
		{Title: "Cluster", Detail: clusterSummary()},
		{Title: "Keys", Detail: "Kit Pro license " + string(status.State)},
		{Title: "Machines", Detail: selfHostLabel()},
		{Title: "Config", Detail: "Sources use this host’s Kit configuration."},
	}
	if selected, err := currentWorkspace(); err == nil {
		cfg.Settings[3].Detail = fmt.Sprintf("Workspace %s · configured Board, Evidence, Orders and Peers sources", selected.Name)
		cfg.Evidence = serveEvidence(evidenceWorkspaceRoot(selected))
		cfg.Orders = serveOrders(selected)
		cfg.Peers = servePeers(selected)
	}
	return server.WithBoard(serveBoard(out)).WithWorkspace(cfg)
}

func clusterSummary() string {
	cfg, err := cluster.LoadConfig("")
	switch {
	case err != nil:
		return "Configuration unavailable"
	case cfg == nil:
		return "Disabled"
	default:
		return "Configured"
	}
}

func serveEvidence(root string) serve.WorkspaceSource {
	return func(ctx context.Context, q string) ([]serve.WorkspaceRow, error) {
		store, err := evidence.Open(root)
		if err != nil {
			return nil, err
		}
		var summaries []evidence.Summary
		if q == "" {
			summaries, err = store.List(evidence.Filter{})
			if err != nil {
				return nil, err
			}
		} else {
			hits, err := store.Search(ctx, evidence.SearchQuery{Q: q, Limit: 50})
			if err != nil {
				return nil, err
			}
			for _, hit := range hits.Data {
				summaries = append(summaries, hit.Package)
			}
		}
		rows := make([]serve.WorkspaceRow, 0, min(200, len(summaries)))
		for _, entry := range summaries[:min(200, len(summaries))] {
			rows = append(rows, serve.WorkspaceRow{ID: entry.ID, Title: entry.Title, Detail: entry.State + " · " + entry.Summary})
		}
		return rows, nil
	}
}

func serveOrders(selected workspace.Selection) serve.WorkspaceSource {
	return func(ctx context.Context, q string) ([]serve.WorkspaceRow, error) {
		var query *grep.Query
		if q != "" {
			var err error
			query, err = grep.Compile([]string{q})
			if err != nil {
				return nil, err
			}
		}
		rows := []serve.WorkspaceRow{}
		err := dispatch.VisitWorkspaceOrders(ctx, selected.Name, func(name string, meta dispatch.Meta) bool {
			detail := meta.Repo + " · " + meta.Host
			if query != nil {
				if _, matches := query.Score(name + " " + detail); !matches {
					return true
				}
			}
			rows = append(rows, serve.WorkspaceRow{ID: name, Title: name, Detail: detail})
			return len(rows) < 200
		})
		return rows, err
	}
}

func servePeers(selected workspace.Selection) serve.WorkspaceSource {
	return func(ctx context.Context, _ string) ([]serve.WorkspaceRow, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var output bytes.Buffer
		if err := peers.New().Run(ctx, []string{"ls", "--json"}, nil, &output, io.Discard); err != nil {
			return nil, err
		}
		var inventory struct{ Panes []peers.Peer }
		if err := json.Unmarshal(output.Bytes(), &inventory); err != nil {
			return nil, err
		}
		visibleRuns := make(map[string]bool)
		if err := dispatch.VisitWorkspaceOrders(ctx, selected.Name, func(name string, _ dispatch.Meta) bool {
			visibleRuns[name] = true
			return true
		}); err != nil {
			return nil, err
		}
		rows := make([]serve.WorkspaceRow, 0, min(200, len(inventory.Panes)))
		for _, peer := range inventory.Panes {
			if peer.Run && !visibleRuns[peer.Session] {
				continue
			}
			rows = append(rows, serve.WorkspaceRow{Title: peer.Address, Detail: peer.Agent + " · " + peer.State})
			if len(rows) == 200 {
				break
			}
		}
		return rows, nil
	}
}

func filterWorkspaceRows(rows []serve.WorkspaceRow, q string) ([]serve.WorkspaceRow, error) {
	if q == "" {
		return rows, nil
	}
	query, err := grep.Compile([]string{q})
	if err != nil {
		return nil, err
	}
	filtered := rows[:0]
	for _, row := range rows {
		if _, matches := query.Score(row.Title + " " + row.Detail); matches {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func hostedWorkspace(c *kitapi.Client, keys []string) serve.Workspace {
	return serve.Workspace{
		Tier: serve.Hosted, Secrets: keys,
		Evidence: hostedWorkspaceSource(c, "evidence"),
		Orders:   hostedWorkspaceSource(c, "orders"),
		Peers:    hostedWorkspaceSource(c, "peers"),
		Settings: []serve.WorkspaceRow{
			{Title: "Billing", Detail: "Manage billing in the hosted workspace."},
			{Title: "Team", Detail: "Manage members in the hosted workspace."},
			{Title: "API Keys", Detail: "Manage API keys in the hosted workspace."},
			{Title: "Machines", Detail: "Manage machines in the hosted workspace."},
		},
	}
}

func hostedWorkspaceSource(c *kitapi.Client, section string) serve.WorkspaceSource {
	type entry struct {
		ID, Title, Name, Summary, Repo, Host, State, Address string
	}
	return func(ctx context.Context, q string) ([]serve.WorkspaceRow, error) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		query := url.Values{"limit": {"100"}}
		var rows []serve.WorkspaceRow
		// Bound pagination even if a server returns a cursor that never advances.
		for range 10 {
			var page struct {
				Data []entry `json:"data"`
				Next string  `json:"next_cursor"`
			}
			if err := c.Do(ctx, http.MethodGet, "/v1/"+section, query, nil, &page); err != nil {
				return nil, err
			}
			for _, item := range page.Data {
				title := item.Title
				if title == "" {
					title = item.Name
				}
				if title == "" {
					title = item.Address
				}
				if title == "" {
					title = item.ID
				}
				detail := strings.Trim(strings.Join([]string{item.State, item.Summary, item.Repo, item.Host}, " "), " ")
				rows = append(rows, serve.WorkspaceRow{ID: item.ID, Title: title, Detail: detail})
			}
			if page.Next == "" {
				return filterWorkspaceRows(rows, q)
			}
			if page.Next == query.Get("cursor") {
				return nil, errors.New("source cursor did not advance")
			}
			query.Set("cursor", page.Next)
		}
		return filterWorkspaceRows(rows, q)
	}
}
