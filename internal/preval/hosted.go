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

func (h HostedDocuments) request(ctx context.Context, method, prefix, id, revision string, body []byte) (*http.Response, error) {
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
	} else if prefix == h.TracePath || prefix == h.OrderPath {
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
	response, err := h.request(ctx, http.MethodGet, prefix, id, "", nil)
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

func (h HostedDocuments) write(ctx context.Context, prefix, id string, eval Evaluation) error {
	for range 2 {
		doc, revision, err := h.read(ctx, prefix, id)
		if err != nil {
			return err
		}
		fields := make(map[string]json.RawMessage)
		if doc.Evaluation != nil {
			fields["evaluation"], _ = json.Marshal(doc.Evaluation)
		}
		changed, err := Patch(fields, eval)
		if err != nil || !changed {
			return err
		}
		response, err := h.request(ctx, http.MethodPut, prefix, id, revision, fields["evaluation"])
		if err != nil {
			return err
		}
		response.Body.Close()
		switch response.StatusCode {
		case http.StatusOK, http.StatusNoContent:
			return nil
		case http.StatusPreconditionFailed:
			continue
		default:
			return errPersistence
		}
	}
	return ErrConflict
}

func (h HostedDocuments) ReadOrder(ctx context.Context, p Provenance) (*Evaluation, error) {
	doc, _, err := h.read(ctx, h.OrderPath, p.OrderID)
	return doc.Evaluation, err
}

func (h HostedDocuments) WriteOrder(ctx context.Context, eval Evaluation) error {
	return h.write(ctx, h.OrderPath, eval.Provenance.OrderID, eval)
}

func (h HostedDocuments) WriteTrace(ctx context.Context, id string, eval Evaluation) error {
	return h.write(ctx, h.TracePath, id, eval)
}

func (h HostedDocuments) WriteEvidence(ctx context.Context, id string, eval Evaluation) error {
	return h.write(ctx, h.EvidencePath, id, eval)
}
