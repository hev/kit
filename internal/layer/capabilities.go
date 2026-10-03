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
// The gateway declares wire features at GET /v2/namespaces/{ns}/capabilities.
// StaticCapabilities retains compatibility answers for the other client seams.
type Capabilities struct {
	// Declared is false when the running gateway has no declaration for the
	// store. Such an answer carries no meaning and is never trusted: see
	// ResolveCapabilities.
	Declared     bool                  `json:"declared"`
	Store        StoreRef              `json:"store"`
	Features     []FeatureCoverage     `json:"features"`
	HybridRoutes []HybridRouteCoverage `json:"hybrid_routes"`
	SchemaLimits SchemaLimits          `json:"schema_limits"`
	// ArrayAttributes is whether the store takes `[]string` and `[]uint`
	// attribute types. The gateway declares no feature for it; a store
	// without them answers 422 with feature "schema.type".
	ArrayAttributes Support
}

type StoreRef struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
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
	Support Support `json:"support"`
	Note    string  `json:"note"`
}

// FeatureCoverage is one row of the gateway's wire-feature inventory. kit lists
// only the features it branches on.
type FeatureCoverage struct {
	ID      string  `json:"id"`
	Support Support `json:"support"`
	Note    string  `json:"note"`
}

// Feature ids are the gateway's (vectorstore_core WireFeature ids).
const (
	// FeatureConditionalWrites is `upsert_condition` on a write.
	FeatureConditionalWrites = "conditional_writes"
	// FeatureOrderedScan is a query ranked by an attribute or by id rather
	// than by relevance: every listing the read side makes.
	FeatureOrderedScan = "ordered_scan"
	// FeaturePatchRows is `patch_rows` on a write: the session summary
	// backfill.
	FeaturePatchRows = "patch_rows"
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
	Route   HybridRoute `json:"route"`
	Support Support     `json:"support"`
	Note    string      `json:"note"`
}

// SchemaLimits bounds a namespace schema declaration. A nil max is unbounded.
type SchemaLimits struct {
	Embed                     Coverage `json:"embed"`
	MaxGatewayEmbedAttributes *int     `json:"max_gateway_embed_attributes"`
	MaxFullTextSearchFields   *int     `json:"max_full_text_search_fields"`
	MaxVectorFields           *int     `json:"max_vector_fields"`
}

// Store kinds with a static answer. The empty kind is a config that names no
// store, which is every config written before `hev up`: the hosted lane.
const (
	StoreTurbopuffer = "turbopuffer"
	StorePgvector    = "pgvector"
)

func limit(n int) *int { return &n }

// StaticCapabilities is the table the runtime read replaces. The pgvector row
// copies what layer-gateway:0.7.2 declares for the store
// (vectorstore-core/src/pgvector_capabilities.rs), feature by feature, for
// the features kit branches on:
//
//   - multi_query unsupported, hybrid_text approximate: fuzziness 0 only (LYR-85)
//   - ordered_scan and conditional_writes come only from runtime declarations
//     (LYR-112); older or undeclared servers enable neither
//   - row patches supported (LYR-140); before 0.7.2 patch_rows was a 422
//     there, so the summary patch still asks the gateway its version (see
//     Client.SummariesServed)
//   - any number of full-text fields, one vector field (LYR-87)
//   - embed approximate with one gateway-embedded attribute: the gateway
//     embeds `text` for Postgres at write and query time with the bundled CPU
//     sidecar as the provider (LYR-88, layer-pro RFC 0118 steps C and E)
//   - array attributes, with Contains/ContainsAny on them (LYR-138), and
//     exclude_attributes on every query (LYR-137); before 0.7.1 an array type
//     was a 422 schema.type there (see layer.MigrateSessionLists)
func StaticCapabilities(kind string) (Capabilities, error) {
	switch kind {
	case "", StoreTurbopuffer:
		return Capabilities{
			Declared: true,
			Store:    StoreRef{Kind: StoreTurbopuffer},
			Features: []FeatureCoverage{
				{ID: FeatureConditionalWrites, Support: Supported},
				{ID: FeatureOrderedScan, Support: Supported},
				{ID: FeaturePatchRows, Support: Supported},
			},
			HybridRoutes: []HybridRouteCoverage{
				{Route: RouteHybridText, Support: Supported},
				{Route: RouteMultiQuery, Support: Supported},
			},
			SchemaLimits:    SchemaLimits{Embed: Coverage{Support: Supported}},
			ArrayAttributes: Supported,
		}, nil
	case StorePgvector:
		return Capabilities{
			Declared: true,
			Store:    StoreRef{Kind: StorePgvector},
			Features: []FeatureCoverage{
				{ID: FeaturePatchRows, Support: Supported},
			},
			HybridRoutes: []HybridRouteCoverage{
				{Route: RouteHybridText, Support: Approximate, Note: "fuzziness 0 only (BM25 + dense legs, gateway RRF); auto/1/2 fuzziness and cursor/temporal_filter return 422"},
				{Route: RouteMultiQuery, Support: Unsupported, Note: "422 for a queries or rerank_by body; hybrid retrieval is the HybridText rank operator"},
			},
			SchemaLimits: SchemaLimits{
				Embed:                     Coverage{Support: Approximate, Note: "gateway-resolved embedding only; one embedded attribute per namespace; chunked embedding returns 422"},
				MaxGatewayEmbedAttributes: limit(1),
				MaxVectorFields:           limit(1),
			},
			ArrayAttributes: Supported,
		}, nil
	}
	return Capabilities{}, fmt.Errorf("unknown layer store %q (expected %s or %s)", kind, StoreTurbopuffer, StorePgvector)
}

// ResolveCapabilities consumes pgvector's ordered/conditional declarations while
// preserving the remaining compatibility answers. Missing features never enable
// operations. The server's support and reason travel together, including refusals.
func ResolveCapabilities(runtime *Capabilities, kind string) (Capabilities, error) {
	caps, err := StaticCapabilities(kind)
	if err != nil {
		return Capabilities{}, err
	}
	if runtime == nil || !runtime.Declared {
		return caps, nil
	}
	if kind != StorePgvector {
		return *runtime, nil
	}
	if runtime.Store.Kind != kind {
		return caps, nil
	}
	caps.Store = runtime.Store
	for _, id := range []string{FeatureOrderedScan, FeatureConditionalWrites} {
		for _, feature := range runtime.Features {
			if feature.ID == id {
				caps.Features = append(caps.Features, feature)
				break
			}
		}
	}
	return caps, nil
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
// written and never read back. The write path skips them instead. It is a
// re-index, not a migration, when the store gains the scan:
// `hev index --force --read-side`.
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

// HybridTextOptions is the fourth element of a HybridText or Auto rank
// expression, or nil for the gateway's defaults. An approximate store serves
// the phase-one subset only: exact-match legs, and no cursor or
// temporal_filter in the body. Postgres is one, and there the default
// fuzziness is a 422 on every input Auto routes to a lexical leg: one or two
// tokens (hybrid_text), and three to seven (fused).
// kit sends neither of the last two on any route, and hybridTextBody keeps it
// that way.
func (c Capabilities) HybridTextOptions() map[string]any {
	if c.route(RouteHybridText) == Approximate {
		return map[string]any{"fuzziness": 0}
	}
	return nil
}

// CanEmbed reports whether a schema may declare `embed` on the one text
// column kit embeds: the store embeds, natively or through the gateway, and
// its gateway-embedded attribute limit leaves room for one. kit computes no
// vectors on any lane: where this is false the field is omitted and retrieval
// is lexical, and when the store gains embedding the declaration returns with
// no second client change.
func (c Capabilities) CanEmbed() bool {
	n := c.SchemaLimits.MaxGatewayEmbedAttributes
	return c.SchemaLimits.Embed.Support.usable() && (n == nil || *n > 0)
}

// Arrays reports whether array attributes can be declared. Where they cannot,
// kit stores its two list columns as JSON strings, as it does tool_counts on
// every store, and reads either form back (trace.UintList, trace.StringList).
// What that costs is filtering on them in the store: ContainsAny on
// tool_names.
func (c Capabilities) Arrays() bool { return c.ArrayAttributes.usable() }

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
		embed := map[string]any{"model": c.Model}
		if cpuModels[c.Model] > 0 {
			embed["serving"] = map[string]any{"prefer": "local"}
		}
		f["embed"] = embed
	}
	return f
}
