package foh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultLayerDocsURL is the documentation root for hev layer.
	DefaultLayerDocsURL = "https://hevlayer.com/docs"
	// DefaultMindDocsURL is the site root for hev mind (creator, company, brand, writing).
	DefaultMindDocsURL = "https://hevmind.com"
)

type docsArgs struct {
	Site   string `json:"site"`   // kit (default) | layer | mind
	Action string `json:"action"` // index | read
	Page   string `json:"page"`   // slug or path, e.g. "dispatch" or "/docs/dispatch.md"
}

func (t *Toolset) docsBaseURL(site string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(site)) {
	case "", "kit", "hevkit":
		base := strings.TrimRight(t.DocsURL, "/")
		if base == "" {
			base = strings.TrimRight(DefaultDocsURL, "/")
		}
		return "kit", base, nil
	case "layer", "hevlayer":
		return "layer", strings.TrimRight(DefaultLayerDocsURL, "/"), nil
	case "mind", "hevmind":
		return "mind", strings.TrimRight(DefaultMindDocsURL, "/"), nil
	default:
		return "", "", fmt.Errorf("unknown docs site %q (use kit, layer, or mind)", site)
	}
}

func (t *Toolset) docsTool() Tool {
	return Tool{
		Spec: FunctionSpec{
			Name:        "hev_docs",
			Description: "Fetch documentation from hevkit.com/docs (site=kit, default), hevlayer.com/docs (site=layer), or hevmind.com (site=mind, for questions about the creator, company, brand, or writing). action=index reads llms.txt; action=read with page reads a specific slug or path from llms.txt.",
			Parameters:  schema(`{"type":"object","properties":{"site":{"type":"string","enum":["kit","layer","mind"]},"action":{"type":"string","enum":["index","read"]},"page":{"type":"string"}},"required":["action"]}`),
		},
		Timeout: 20 * time.Second,
		Run: func(ctx context.Context, raw json.RawMessage) ToolResult {
			var a docsArgs
			_ = json.Unmarshal(raw, &a)
			card := &Card{Tool: "hev_docs", Action: a.Action}
			siteName, base, err := t.docsBaseURL(a.Site)
			if err != nil {
				card.Summary = err.Error()
				return ToolResult{Content: "hev_docs: " + err.Error(), IsError: true, Card: card}
			}
			llms := base + "/llms.txt"
			if strings.HasSuffix(base, "/docs") {
				llms = strings.TrimSuffix(base, "/docs") + "/llms.txt"
			}
			prefix := ""
			if siteName != "kit" {
				prefix = siteName + " "
			}
			var target string
			switch a.Action {
			case "index", "":
				card.Action = prefix + "index"
				target = llms
			case "read":
				page := strings.TrimSpace(a.Page)
				if page == "" {
					card.Summary = "page is required"
					return ToolResult{Content: "hev_docs: page is required for read", IsError: true, Card: card}
				}
				card.Action = prefix + "read " + truncateLine(page, 32)
				resolved, err := resolveDocsTarget(base, page)
				if err != nil {
					card.Summary = err.Error()
					return ToolResult{Content: "hev_docs: " + err.Error(), IsError: true, Card: card}
				}
				target = resolved
			default:
				card.Summary = "unknown action " + a.Action
				return ToolResult{Content: fmt.Sprintf("hev_docs: unknown action %q", a.Action), IsError: true, Card: card}
			}

			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				card.Summary = "invalid docs url"
				return ToolResult{Content: "hev_docs: invalid docs url", IsError: true, Card: card}
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				card.Summary = "fetch failed"
				return ToolResult{Content: "hev_docs: " + err.Error(), IsError: true, Card: card}
			}
			defer resp.Body.Close()
			bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
			if err != nil {
				card.Summary = "read failed"
				return ToolResult{Content: "hev_docs: " + err.Error(), IsError: true, Card: card}
			}
			body := strings.TrimSpace(string(bodyBytes))
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				card.Summary = fmt.Sprintf("HTTP %d", resp.StatusCode)
				return ToolResult{Content: fmt.Sprintf("hev_docs: HTTP %d from %s", resp.StatusCode, target), IsError: true, Card: card}
			}
			card.OK = true
			card.Body = body
			if strings.HasSuffix(card.Action, "index") {
				card.Summary = fmt.Sprintf("loaded %s docs index (%d lines)", siteName, strings.Count(body, "\n")+1)
			} else {
				card.Summary = fmt.Sprintf("loaded %s %s (%d bytes)", siteName, a.Page, len(body))
			}
			return ToolResult{Content: body, Card: card}
		},
	}
}

// resolveDocsTarget resolves a page slug, root-relative path, or same-origin URL
// against the configured docs base URL, refusing off-origin URLs.
func resolveDocsTarget(base, page string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil || baseURL.Host == "" {
		return "", fmt.Errorf("invalid docs base URL")
	}
	if strings.HasPrefix(page, "http://") || strings.HasPrefix(page, "https://") {
		u, err := url.Parse(page)
		if err != nil {
			return "", fmt.Errorf("invalid page URL")
		}
		if !strings.EqualFold(u.Host, baseURL.Host) {
			return "", fmt.Errorf("refusing off-origin docs host %q", u.Host)
		}
		return u.String(), nil
	}
	if strings.HasPrefix(page, "/") {
		return baseURL.Scheme + "://" + baseURL.Host + page, nil
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(page, "/"), nil
}
