package preval

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HostedDocuments uses tenant-scoped resource URLs from the existing service.
type HostedDocuments struct {
	BaseURL      string
	OrderPath    string
	TracePath    string
	EvidencePath string
	Token        string
	client       *http.Client
}

type hostedDocument struct {
	Evaluation *Evaluation `json:"evaluation"`
}

func (h HostedDocuments) request(ctx context.Context, method, prefix, id, revision string, body []byte, metadataOnly bool) (*http.Response, error) {
	base, err := url.Parse(h.BaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil ||
		base.RawQuery != "" || base.Fragment != "" || h.Token == "" ||
		!strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "//") || strings.ContainsAny(prefix, "?#") ||
		!identifier.MatchString(id) {
		return nil, errInvalid
	}
	path := strings.TrimRight(h.BaseURL, "/") + strings.TrimRight(prefix, "/") + "/" + url.PathEscape(id)
	if method == http.MethodPut {
		path += "/evaluation"
	} else if !metadataOnly && prefix == h.TracePath {
		path += "?view=evaluation&sample=transcript"
	} else if metadataOnly && (prefix == h.TracePath || prefix == h.OrderPath) {
		path += "?view=evaluation"
	}
	request, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return nil, errInvalid
	}
	request.Header.Set("Authorization", "Bearer "+h.Token)
	if revision != "" {
		request.Header.Set("If-Match", revision)
		request.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{Timeout: providerTimeout}
	if h.client != nil {
		client = *h.client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, errPersistence
	}
	return response, nil
}

func (h HostedDocuments) read(ctx context.Context, prefix, id string) (hostedDocument, string, error) {
	response, err := h.request(ctx, http.MethodGet, prefix, id, "", nil, true)
	if err != nil {
		return hostedDocument{}, "", err
	}
	defer response.Body.Close()
	revision := response.Header.Get("ETag")
	if response.StatusCode != http.StatusOK || len(revision) < 2 || revision[0] != '"' || revision[len(revision)-1] != '"' {
		return hostedDocument{}, "", errPersistence
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPayload+1))
	var doc hostedDocument
	if err != nil || len(body) > maxPayload || json.Unmarshal(body, &doc) != nil {
		return hostedDocument{}, "", errPersistence
	}
	return doc, revision, nil
}

func (h HostedDocuments) write(ctx context.Context, prefix, id string, eval Evaluation) (bool, error) {
	for range 2 {
		doc, revision, err := h.read(ctx, prefix, id)
		if err != nil {
			return false, err
		}
		fields := make(map[string]json.RawMessage)
		if doc.Evaluation != nil {
			fields["evaluation"], _ = json.Marshal(doc.Evaluation)
		}
		changed, err := Patch(fields, eval)
		if err != nil || !changed {
			return false, err
		}
		response, err := h.request(ctx, http.MethodPut, prefix, id, revision, fields["evaluation"], true)
		if err != nil {
			return false, err
		}
		response.Body.Close()
		switch response.StatusCode {
		case http.StatusOK, http.StatusNoContent:
			return true, nil
		case http.StatusPreconditionFailed:
			continue
		default:
			return false, errPersistence
		}
	}
	return false, ErrConflict
}

func (h HostedDocuments) ReadOrder(ctx context.Context, p Provenance) (*Evaluation, error) {
	doc, _, err := h.read(ctx, h.OrderPath, p.OrderID)
	return doc.Evaluation, err
}

func (h HostedDocuments) WriteOrder(ctx context.Context, eval Evaluation) error {
	_, err := h.write(ctx, h.OrderPath, eval.Provenance.OrderID, eval)
	return err
}

func (h HostedDocuments) WriteTrace(ctx context.Context, id string, eval Evaluation) error {
	_, err := h.write(ctx, h.TracePath, id, eval)
	return err
}

func (h HostedDocuments) WriteEvidence(ctx context.Context, id string, eval Evaluation) error {
	_, err := h.write(ctx, h.EvidencePath, id, eval)
	return err
}

func (h HostedDocuments) ClaimLearning(ctx context.Context, eval Evaluation) (bool, error) {
	if eval.Learning.Status != "failed" {
		return false, ErrConflict
	}
	return h.write(ctx, h.OrderPath, eval.Provenance.OrderID, eval)
}

// LearningMaterial samples only authorized source documents carrying the same
// attributed observation. It is called after all projections have landed.
func (h HostedDocuments) LearningMaterial(ctx context.Context, eval Evaluation) ([]Excerpt, error) {
	p := eval.Provenance
	excerpts := make([]Excerpt, 0, 11)
	appendSource := func(prefix, id, kind string) error {
		response, err := h.request(ctx, http.MethodGet, prefix, id, "", nil, false)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return errPersistence
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maxPayload+1))
		var doc struct {
			Evaluation *Evaluation
			Order      string
			Summary    string
			Sample     string
			Claims     []struct {
				Criterion string
				Text      string
			}
		}
		// On evidence, "order" is its order ID. On Orders, it is the order
		// document. Both are bounded by the eventual excerpt budget.
		if err != nil || len(body) > maxPayload || json.Unmarshal(body, &doc) != nil ||
			doc.Evaluation == nil || doc.Evaluation.ID != eval.ID ||
			!doc.Evaluation.ObservedAt.Equal(eval.ObservedAt) {
			return errPersistence
		}
		const budget = 4800
		var sample strings.Builder
		add := func(text string) {
			if sample.Len() < budget && text != "" {
				sample.WriteString(text[:min(len(text), budget-sample.Len()-1)])
				sample.WriteByte('\n')
			}
		}
		switch kind {
		case "order":
			add(doc.Order)
		case "trace":
			add(doc.Sample)
		case "evidence":
			add(doc.Summary)
			for _, claim := range doc.Claims[:min(8, len(doc.Claims))] {
				add(claim.Criterion + ": " + claim.Text)
			}
		}
		excerpts = append(excerpts, Excerpt{Reference: kind + ":" + id, Text: sample.String()})
		return nil
	}
	if err := appendSource(h.OrderPath, p.OrderID, "order"); err != nil {
		return nil, err
	}
	for _, id := range p.TraceSessionIDs[:min(5, len(p.TraceSessionIDs))] {
		if err := appendSource(h.TracePath, id, "trace"); err != nil {
			return nil, err
		}
	}
	for _, id := range p.EvidenceIDs[:min(5, len(p.EvidenceIDs))] {
		if err := appendSource(h.EvidencePath, id, "evidence"); err != nil {
			return nil, err
		}
	}
	return excerpts, nil
}
