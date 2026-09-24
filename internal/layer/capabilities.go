package layer

import (
	"fmt"
	"os"
	"strings"
)

// Capabilities answers "what can this store do". It is the one place kit
// decides anything per backend: the schema builders, the eval schema, and the
// search path all consult it, and nothing else in the client branches on a
// store kind.
//
// The shape follows the gateway's expected runtime read (layer-pro RFC 0117,
// GET /v2/namespaces/{ns}/capabilities — not served by any gateway yet), so
// that read can fill this struct without a rename. Until it exists the answers
// come from the static table in StaticCapabilities.
type Capabilities struct {
	// Declared is false when the running gateway has no declaration for the
	// store. Such an answer carries no meaning and is never trusted: see
	// ResolveCapabilities.
	Declared     bool
	Store        StoreRef
	Features     []FeatureCoverage
	HybridRoutes []HybridRouteCoverage
	SchemaLimits SchemaLimits
}

type StoreRef struct {
	Name string
	Kind string
}

// Support is the gateway's closed four-value set.
type Support string

const (
	Supported   Support = "supported"
	Approximate Support = "approximate"
	Unsupported Support = "unsupported"
	Undeclared  Support = "undeclared"
)

// usable reports whether a feature can be called at all. Anything outside the
// closed set, including Undeclared, is not a yes.
func (s Support) usable() bool { return s == Supported || s == Approximate }

type Coverage struct {
	Support Support
	Note    string
}

// FeatureCoverage is one row of the gateway's wire-feature inventory. kit lists
// only the features it branches on.
type FeatureCoverage struct {
	ID      string
	Support Support
	Note    string
}

// Feature ids are the gateway's (vectorstore_core WireFeature ids).
const (
	// FeatureConditionalWrites is `upsert_condition` on a write.
	FeatureConditionalWrites = "conditional_writes"
	// FeatureOrderedScan is a query ranked by an attribute or by id rather
	// than by relevance: every listing the read side makes.
	FeatureOrderedScan = "ordered_scan"
)

// HybridRoute names the two ways a store can serve hybrid retrieval. The ids
// are the gateway's (vectorstore_core HybridRoute::id()).
type HybridRoute string

const (
	// RouteMultiQuery is the native passthrough: {"queries": [...],
	// "rerank_by": ["RRF"]}, fused by the store.
	RouteMultiQuery HybridRoute = "multi_query"
	// RouteHybridText is the gateway's HybridText rank operator: one
	// expression in, legs issued and fused gateway-side.
	RouteHybridText HybridRoute = "hybrid_text"
)

type HybridRouteCoverage struct {
	Route   HybridRoute
	Support Support
	Note    string
}

// SchemaLimits bounds a namespace schema declaration. A nil max is unbounded.
type SchemaLimits struct {
	Embed                     Coverage
	MaxGatewayEmbedAttributes *int
	MaxFullTextSearchFields   *int
	MaxVectorFields           *int
}

// Store kinds with a static answer. The empty kind is a config that names no
// store, which is every config written before `hev up`: the hosted lane.
const (
	StoreTurbopuffer = "turbopuffer"
	StorePgvector    = "pgvector"
)

func limit(n int) *int { return &n }

// StaticCapabilities is the table the runtime read replaces. The pgvector row
// records the store as it behaves on layer-gateway:edge today; each entry is a
// workaround for an open Layer issue and disappears when that issue's answer
// arrives through ResolveCapabilities:
//
//   - multi_query unsupported, hybrid_text approximate (LYR-85)
//   - embed unsupported (LYR-88)
//   - one full-text field (LYR-87)
//   - no conditional writes, no ordered scan (found by this work; no issue yet)
func StaticCapabilities(kind string) (Capabilities, error) {
	switch kind {
	case "", StoreTurbopuffer:
		return Capabilities{
			Declared: true,
			Store:    StoreRef{Kind: StoreTurbopuffer},
			Features: []FeatureCoverage{
				{ID: FeatureConditionalWrites, Support: Supported},
				{ID: FeatureOrderedScan, Support: Supported},
			},
			HybridRoutes: []HybridRouteCoverage{
				{Route: RouteHybridText, Support: Supported},
				{Route: RouteMultiQuery, Support: Supported},
			},
			SchemaLimits: SchemaLimits{Embed: Coverage{Support: Supported}},
		}, nil
	case StorePgvector:
		return Capabilities{
			Declared: true,
			Store:    StoreRef{Kind: StorePgvector},
			Features: []FeatureCoverage{
				{ID: FeatureConditionalWrites, Support: Unsupported, Note: "422 for upsert_condition"},
				{ID: FeatureOrderedScan, Support: Unsupported, Note: "422 for rank_by on an attribute or id, and for a filter-only query"},
			},
			HybridRoutes: []HybridRouteCoverage{
				{Route: RouteHybridText, Support: Approximate, Note: "phase one: fuzziness 0, no cursor, no temporal_filter"},
				{Route: RouteMultiQuery, Support: Unsupported, Note: "422 for a queries or rerank_by body; hybrid retrieval is the HybridText rank operator"},
			},
			SchemaLimits: SchemaLimits{
				Embed:                     Coverage{Support: Unsupported},
				MaxGatewayEmbedAttributes: limit(0),
				MaxFullTextSearchFields:   limit(1),
				MaxVectorFields:           limit(1),
			},
		}, nil
	}
	return Capabilities{}, fmt.Errorf("unknown layer store %q (expected %s or %s)", kind, StoreTurbopuffer, StorePgvector)
}

// ResolveCapabilities is where a runtime answer meets the static table. An
// undeclared answer falls back to the table for the configured kind; it is
// never read as "supported".
func ResolveCapabilities(runtime *Capabilities, kind string) (Capabilities, error) {
	if runtime != nil && runtime.Declared {
		return *runtime, nil
	}
	return StaticCapabilities(kind)
}

// StoreKind reads the configured store: LAYER_STORE, else the config value.
func StoreKind(configured string) string {
	if v := os.Getenv("LAYER_STORE"); v != "" {
		return strings.ToLower(v)
	}
	return strings.ToLower(configured)
}

// Feature is the store's answer for one wire feature; Undeclared when the
// inventory does not mention it.
func (c Capabilities) Feature(id string) Support {
	for _, f := range c.Features {
		if f.ID == id {
			return f.Support
		}
	}
	return Undeclared
}

// WriteCondition returns the upsert_condition to send, or nil where the store
// rejects one. The conditions kit sends guard against replays — an older
// transcript rolling a session row back, an eval written twice — and a write
// without them is last-writer-wins. That is a weaker guarantee, and it is the
// store's: kit does not emulate the condition with a read-modify-write.
func (c Capabilities) WriteCondition(condition any) any {
	if c.Feature(FeatureConditionalWrites).usable() {
		return condition
	}
	return nil
}

// ReadSide reports whether the store can serve the blocks and sessions
// namespaces. Both are only ever read by ordered scan — newest sessions first,
// a session's blocks by seq — so where that is missing the rows could be
// written and never read back, and their schema (array columns, patches) is
// more the store would have to accept. The write path skips them instead, and
// search is all the archive offers. It is a re-index, not a migration, when
// the store gains the scan: `hev index --force --read-side`.
func (c Capabilities) ReadSide() bool { return c.Feature(FeatureOrderedScan).usable() }

func (c Capabilities) route(r HybridRoute) Support {
	for _, h := range c.HybridRoutes {
		if h.Route == r {
			return h.Support
		}
	}
	return Undeclared
}

// SearchRoute picks how hybrid retrieval is asked for. The native multi-query
// is preferred wherever the store serves it, because that is the body hosted
// archives have always been searched with; HybridText is the route for a store
// that does not.
func (c Capabilities) SearchRoute() (HybridRoute, error) {
	if c.route(RouteMultiQuery).usable() {
		return RouteMultiQuery, nil
	}
	if c.route(RouteHybridText).usable() {
		return RouteHybridText, nil
	}
	return "", fmt.Errorf("layer store %q serves no hybrid retrieval route", c.Store.Kind)
}

// HybridTextOptions is the fourth element of a HybridText rank expression, or
// nil for the gateway's defaults. An approximate store serves the phase-one
// subset only: exact-match legs, and no cursor or temporal_filter in the body.
// kit sends neither of the last two on any route, and hybridTextBody keeps it
// that way.
func (c Capabilities) HybridTextOptions() map[string]any {
	if c.route(RouteHybridText) == Approximate {
		return map[string]any{"fuzziness": 0}
	}
	return nil
}

// CanEmbed reports whether a schema may declare `embed`. kit computes no
// vectors on any lane: where this is false the field is omitted and retrieval
// is lexical, and when the store gains embedding the declaration returns with
// no second client change.
func (c Capabilities) CanEmbed() bool { return c.SchemaLimits.Embed.Support.usable() }

// FullTextFields keeps as many of wanted, in priority order, as the store
// indexes for full-text search.
func (c Capabilities) FullTextFields(wanted ...string) map[string]bool {
	if n := c.SchemaLimits.MaxFullTextSearchFields; n != nil && len(wanted) > *n {
		wanted = wanted[:max(0, *n)]
	}
	keep := make(map[string]bool, len(wanted))
	for _, f := range wanted {
		keep[f] = true
	}
	return keep
}

// textField declares a searched text column: full-text always, embedded where
// the store can.
func (c *Client) textField() map[string]any {
	f := map[string]any{"type": "string", "full_text_search": true}
	if c.Caps.CanEmbed() {
		f["embed"] = map[string]any{"model": c.Model}
	}
	return f
}
