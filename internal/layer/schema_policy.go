package layer

// IndexedAttributes is the complete query index policy. IDs are provider built-ins.
// Keep docs/schema-cut/policy.json in sync; additions require a concrete query.
var IndexedAttributes = map[string][]string{
	"sessions":     {"session_id", "harness", "host", "project", "model", "start", "instance", "pr", "end", "repo_url", "tool_names", "tool_count", "total_tokens", "cost", "wall_ms", "commits", "workdir", "branch", "outcome_checked", "summary_backfilled", "skills"},
	"chunks":       {"session_id", "harness", "project", "ts", "plan", "workdir", "seq", "part"},
	"blocks":       {"session_id", "seq"},
	"evals":        {"session_id"},
	"instructions": {"path", "project", "host"},
}

func indexedSchema(schema map[string]any, kind string) map[string]any {
	for _, definition := range schema {
		definition.(map[string]any)["filterable"] = false
	}
	for _, field := range IndexedAttributes[kind] {
		if definition, ok := schema[field]; ok {
			definition.(map[string]any)["filterable"] = true
		}
	}
	return schema
}
