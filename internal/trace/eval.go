package trace

// Eval is a generic grade of a trace. Mark names belong to the producer.
// TS is RFC3339; evidence text may cite the producer's turn identifiers.
type Eval struct {
	Session  string            `json:"session"`
	TS       string            `json:"ts"`
	Role     string            `json:"role"`
	Instance string            `json:"instance"`
	Host     string            `json:"host"`
	Marks    map[string]int    `json:"marks"`
	Poor     bool              `json:"poor"`
	Summary  string            `json:"summary"`
	Evidence map[string]string `json:"evidence"`
	Findings []string          `json:"findings"`
}
