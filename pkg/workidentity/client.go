package workidentity

import (
	"bytes"
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
	requestTimeout = 6 * time.Second
	maxRequest     = 4 << 10
	// A page can contain 100 snapshots with many links apiece.
	maxResponse = 1 << 20
)

type Client struct {
	endpoint  string
	key       string
	workspace UUID
	http      *http.Client
}

// New creates a workspace-scoped client. TLS is required except for local
// loopback fixtures. Redirects are rejected to keep credentials and writes at
// the explicitly configured endpoint.
func New(endpoint, key string, workspace UUID) (*Client, error) {
	if _, err := ParseUUID(string(workspace)); err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("a work identity bearer key is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && localHost(u.Hostname()))) {
		return nil, fmt.Errorf("work identity requires an HTTPS Layer endpoint or a loopback HTTP fixture")
	}
	return &Client{
		endpoint:  strings.TrimRight(u.String(), "/") + "/v2/namespaces/work-" + string(workspace) + "/work-identity",
		key:       key,
		workspace: workspace,
		http: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func localHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Error describes a failed HTTP operation. UnknownCommit requires recovery
// before another write. Only create can be repeated with the original payload.
type Error struct {
	Op            string
	Status        int
	Code          string
	Retryable     bool
	UnknownCommit bool
	cause         error
}

func (e *Error) Error() string {
	message := "work identity " + e.Op + ": " + e.Code
	if e.Status != 0 {
		message += fmt.Sprintf(" (HTTP %d)", e.Status)
	}
	if e.Retryable {
		message += "; retryable failure"
	}
	if e.UnknownCommit {
		message += "; commit outcome unknown"
		if e.Op == "create" {
			message += "; retry with the same idempotency key and payload"
		} else {
			message += "; read current state before retrying the write"
		}
	}
	return message
}

func (e *Error) Unwrap() error { return e.cause }

func call[T any](ctx context.Context, c *Client, op string, write bool, fields any, valid func(T) bool) (T, error) {
	var zero T
	body, err := json.Marshal(fields)
	if err != nil {
		return zero, fmt.Errorf("work identity %s: %w", op, err)
	}
	if len(body) > maxRequest {
		return zero, fmt.Errorf("work identity %s: request exceeds 4 KiB", op)
	}
	target := c.endpoint
	if !write {
		target += "/query"
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("could not construct work identity request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	res, err := c.http.Do(req)
	if err != nil {
		return zero, &Error{Op: op, Code: "Layer response unavailable", Retryable: true, UnknownCommit: write, cause: err}
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var reply struct {
			Code      string `json:"error"`
			Retryable bool   `json:"retryable"`
		}
		err := json.NewDecoder(io.LimitReader(res.Body, maxRequest)).Decode(&reply)
		code := "Layer rejected request"
		// Layer error codes are public enums. Avoid echoing arbitrary remote text.
		if err == nil {
			switch reply.Code {
			case "invalid_request", "unauthorized", "not_found", "conflict", "capacity", "unavailable":
				code = reply.Code
			}
		}
		retryable := res.StatusCode >= 500 || res.StatusCode == http.StatusRequestTimeout ||
			res.StatusCode == http.StatusTooManyRequests || (err == nil && reply.Retryable)
		uncertain := res.StatusCode < 400 || res.StatusCode >= 500 ||
			res.StatusCode == http.StatusRequestTimeout
		return zero, &Error{Op: op, Status: res.StatusCode, Code: code, Retryable: retryable, UnknownCommit: write && uncertain}
	}

	var data T
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err == nil && len(raw) <= maxResponse && json.Unmarshal(raw, &data) == nil && valid(data) {
		return data, nil
	}
	return zero, &Error{Op: op, Status: res.StatusCode, Code: "invalid or incomplete Layer response", UnknownCommit: write, Retryable: err != nil, cause: err}
}

func (c *Client) validAsk(a Ask) bool {
	if a.WorkspaceID != c.workspace || a.ID == "" || !a.Address.valid() || !a.OriginAddress.valid() ||
		a.RequestRef == "" || a.ScopeRef == "" || a.Revision <= 0 || a.ScopeRevision <= 0 ||
		len(a.Segments) == 0 || a.Links == nil {
		return false
	}
	for _, segment := range a.Segments {
		if segment <= 0 {
			return false
		}
	}
	var linkedEvidence bool
	for _, link := range a.Links {
		if !link.Kind.valid() || link.Reference == "" {
			return false
		}
		if a.Acceptance != nil && link.Kind == Artifact && link.Reference == a.Acceptance.EvidenceRef {
			linkedEvidence = true
		}
	}
	return a.Acceptance == nil || (linkedEvidence && a.Acceptance.ScopeRevision == a.ScopeRevision)
}

func (k LinkKind) valid() bool {
	switch k {
	case Attempt, PR, Execution, Artifact:
		return true
	}
	return false
}

// Create sends one attempt. Retry an uncertain result with the exact same
// request, including its caller-supplied idempotency key.
func (c *Client) Create(ctx context.Context, request CreateRequest) (Ask, error) {
	return call(ctx, c, "create", true, struct {
		Op string `json:"op"`
		CreateRequest
	}{"create", request}, func(a Ask) bool {
		return c.validAsk(a) && a.RequestRef == request.RequestRef
	})
}

// Resolve returns the canonical ask for a current address or reserved alias.
func (c *Client) Resolve(ctx context.Context, address Address) (Ask, error) {
	if !address.valid() {
		return Ask{}, fmt.Errorf("expected a work identity address")
	}
	return call(ctx, c, "resolve", false, struct {
		Op      string  `json:"op"`
		Address Address `json:"address"`
	}{"resolve", address}, c.validAsk)
}

func (c *Client) Get(ctx context.Context, id UUID) (Ask, error) {
	return call(ctx, c, "get", false, struct {
		Op string `json:"op"`
		ID UUID   `json:"id"`
	}{"get", id}, c.askFor(id))
}

func (c *Client) askFor(id UUID) func(Ask) bool {
	return func(a Ask) bool { return c.validAsk(a) && a.ID == id }
}

func writeAsk(ctx context.Context, c *Client, op string, target Target, body any, valid func(Ask) bool, allowNoop bool) (Ask, error) {
	if target.ExpectedRevision <= 0 {
		return Ask{}, fmt.Errorf("expected_revision must be positive")
	}
	return call(ctx, c, op, true, body, func(a Ask) bool {
		return c.validAsk(a) && a.ID == target.ID && (a.Revision > target.ExpectedRevision ||
			(allowNoop && a.Revision == target.ExpectedRevision)) && valid(a)
	})
}

func (c *Client) Move(ctx context.Context, request MoveRequest) (Ask, error) {
	if request.Segment <= 0 {
		return Ask{}, fmt.Errorf("segment must be positive")
	}
	return writeAsk(ctx, c, "move", request.Target, struct {
		Op string `json:"op"`
		MoveRequest
	}{"move", request}, func(a Ask) bool {
		return a.Segments[len(a.Segments)-1] == request.Segment &&
			((a.ParentID == nil && request.ParentID == nil) ||
				(a.ParentID != nil && request.ParentID != nil && *a.ParentID == *request.ParentID))
	}, true)
}

func (c *Client) Revise(ctx context.Context, request ReviseRequest) (Ask, error) {
	return writeAsk(ctx, c, "revise", request.Target, struct {
		Op string `json:"op"`
		ReviseRequest
	}{"revise", request}, func(a Ask) bool {
		return a.ScopeRef == request.ScopeRef && a.Acceptance == nil
	}, false)
}

func (c *Client) Link(ctx context.Context, request LinkRequest) (Ask, error) {
	if !request.Kind.valid() {
		return Ask{}, fmt.Errorf("kind must be attempt, pr, execution or artifact")
	}
	return writeAsk(ctx, c, "link", request.Target, struct {
		Op string `json:"op"`
		LinkRequest
	}{"link", request}, func(a Ask) bool {
		for _, link := range a.Links {
			if link == request.Link {
				return true
			}
		}
		return false
	}, true)
}

// Accept records acceptance on the given ask and current scope only. Root and
// child asks each need their own artifact link and evidence.
func (c *Client) Accept(ctx context.Context, request AcceptRequest) (Ask, error) {
	if request.ScopeRevision <= 0 {
		return Ask{}, fmt.Errorf("scope_revision must be positive")
	}
	return writeAsk(ctx, c, "accept", request.Target, struct {
		Op string `json:"op"`
		AcceptRequest
	}{"accept", request}, func(a Ask) bool {
		return a.Acceptance != nil && *a.Acceptance == request.Acceptance
	}, false)
}

func (c *Client) List(ctx context.Context, page PageRequest) (Page[Ask], error) {
	return queryPage(ctx, c, "list", page, c.validAsk)
}

func (c *Client) History(ctx context.Context, page PageRequest) (Page[HistoryItem], error) {
	return queryPage(ctx, c, "history", page, func(item HistoryItem) bool {
		return item.Cursor > page.After && item.Event != "" && c.validAsk(item.Ask)
	})
}

func queryPage[T any](ctx context.Context, c *Client, op string, page PageRequest, valid func(T) bool) (Page[T], error) {
	if page.After < 0 || page.Limit < 0 || page.Limit > 100 {
		return Page[T]{}, fmt.Errorf("expected a nonnegative cursor and a page size from 1 to 100 (0 uses the default)")
	}
	limit := page.Limit
	if limit == 0 {
		limit = 25
	}
	return call(ctx, c, op, false, struct {
		Op string `json:"op"`
		PageRequest
	}{op, page}, func(p Page[T]) bool {
		if p.Items == nil || len(p.Items) > limit {
			return false
		}
		if len(p.Items) == 0 {
			return p.NextAfter == nil && !p.HasMore
		}
		if p.NextAfter == nil || *p.NextAfter <= page.After || (p.HasMore && len(p.Items) != limit) {
			return false
		}
		for _, item := range p.Items {
			if !valid(item) {
				return false
			}
		}
		return true
	})
}
