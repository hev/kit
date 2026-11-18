package layer

// SearchCaps says what an archive's search can do, so commands, the
// dashboard and the skills promise only that. Layer is semantic and hybrid;
// the file archive (no key) is lexical: it matches words, not meaning.
type SearchCaps struct {
	// Semantic is embedding search: a phrasing finds text that means the
	// same thing in other words.
	Semantic bool `json:"semantic"`
	// Lexical is word matching only; true exactly when Semantic is false.
	Lexical bool `json:"lexical"`
	// Regex is --regex: a regular expression over chunk text.
	Regex bool `json:"regex"`
	// Phrase is contiguous-text search (the dashboard's phrase mode).
	Phrase bool `json:"phrase"`
	// Evals is whether evaluations are stored and searched.
	Evals bool `json:"evals"`
	// Engine names what runs the search: "layer", or "rg" / "go" for files.
	Engine string `json:"engine"`
}

// SearchCaps is hybrid: semantic and BM25 legs fused in the store.
func (c *Client) SearchCaps() SearchCaps {
	return SearchCaps{Semantic: true, Phrase: true, Evals: true, Engine: "layer"}
}
