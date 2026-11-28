// Package workidentity is a client for the Layer work identity API.
package workidentity

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// UUID is an opaque identifier or reference. The client never derives it from
// a dispatch run name, address, or storage location.
type UUID string

var canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ParseUUID(value string) (UUID, error) {
	if !canonicalUUID.MatchString(value) {
		return "", fmt.Errorf("expected a canonical UUID")
	}
	return UUID(value), nil
}

func (id UUID) MarshalJSON() ([]byte, error) {
	if _, err := ParseUUID(string(id)); err != nil {
		return nil, err
	}
	return json.Marshal(string(id))
}

func (id *UUID) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseUUID(value)
	if err == nil {
		*id = parsed
	}
	return err
}

// Address is a workspace-relative address assigned by Layer. It can be an old
// alias. Resolve it before taking action on an ask.
type Address string

func (a Address) valid() bool {
	return a != "" && strings.IndexFunc(string(a), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) == -1
}

type LinkKind string

const (
	Attempt   LinkKind = "attempt"
	PR        LinkKind = "pr"
	Execution LinkKind = "execution"
	Artifact  LinkKind = "artifact"
)

type Link struct {
	Kind      LinkKind `json:"kind"`
	Reference UUID     `json:"reference"`
}

type Acceptance struct {
	ScopeRevision int64 `json:"scope_revision"`
	EvidenceRef   UUID  `json:"evidence_ref"`
}

type Ask struct {
	ID             UUID        `json:"id"`
	WorkspaceID    UUID        `json:"workspace_id"`
	Address        Address     `json:"address"`
	OriginAddress  Address     `json:"origin_address"`
	Segments       []int32     `json:"segments"`
	ParentID       *UUID       `json:"parent_id"`
	OriginParentID *UUID       `json:"origin_parent_id"`
	RequestRef     UUID        `json:"request_ref"`
	ScopeRef       UUID        `json:"scope_ref"`
	Revision       int64       `json:"revision"`
	ScopeRevision  int64       `json:"scope_revision"`
	Links          []Link      `json:"links"`
	Acceptance     *Acceptance `json:"acceptance"`
}

type HistoryItem struct {
	Cursor int64  `json:"cursor"`
	Event  string `json:"event"`
	Ask    Ask    `json:"ask"`
}

// PageRequest requests one bounded page. A zero Limit uses Layer's default.
type PageRequest struct {
	After int64 `json:"after,omitempty"`
	Limit int   `json:"limit,omitempty"`
}

type Page[T any] struct {
	Items     []T    `json:"items"`
	NextAfter *int64 `json:"next_after"`
	HasMore   bool   `json:"has_more"`
}

type CreateRequest struct {
	IdempotencyKey UUID  `json:"idempotency_key"`
	ParentID       *UUID `json:"parent_id,omitempty"`
	RequestRef     UUID  `json:"request_ref"`
	ScopeRef       UUID  `json:"scope_ref"`
}

type Target struct {
	ID               UUID  `json:"id"`
	ExpectedRevision int64 `json:"expected_revision"`
}

type MoveRequest struct {
	Target
	ParentID *UUID `json:"parent_id,omitempty"`
	Segment  int32 `json:"segment"`
}

type ReviseRequest struct {
	Target
	ScopeRef UUID `json:"scope_ref"`
}

type LinkRequest struct {
	Target
	Link
}

type AcceptRequest struct {
	Target
	Acceptance
}
