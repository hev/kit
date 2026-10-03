package layer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// BackfillSchema reads only; absent namespaces are an explicit preflight failure.
func (c *Client) BackfillSchema(ctx context.Context) (map[string]json.RawMessage, error) {
	var schema map[string]json.RawMessage
	err := c.doContext(ctx, "GET", "/v1/namespaces/"+c.Namespace+"-sessions/schema", nil, &schema)
	return schema, err
}

// SessionEnrichmentFilterPlan prepares an additive schema-only repair. Calling
// this method does not mutate the archive. Existing definitions retain settings
// and types; missing list types use the configured store's existing capability.
func (c *Client) SessionEnrichmentFilterPlan(ctx context.Context) (map[string]any, error) {
	current, err := c.BackfillSchema(ctx)
	if err != nil {
		return nil, err
	}
	wanted := outcomeSchema(c.Caps.Arrays())
	wanted["pr"] = map[string]any{"type": "string", "filterable": true}
	wanted["commits"] = map[string]any{"type": "string", "filterable": true}
	if c.Caps.Arrays() {
		wanted["commits"].(map[string]any)["type"] = "[]string"
	}
	for k := range current {
		if strings.HasPrefix(k, "ci_workflow_") || strings.HasPrefix(k, "commit_outcome_") {
			wanted[k] = map[string]any{"type": "string", "filterable": true}
		}
	}
	changes := map[string]any{}
	for k, v := range wanted {
		definition := v.(map[string]any)
		if raw, ok := current[k]; ok {
			var stored map[string]any
			if err = json.Unmarshal(raw, &stored); err != nil {
				return nil, err
			}
			typ := stored["type"]
			list := k == "commits" || k == "ci_conclusions" || k == "ci_runs" || k == "revert_commits"
			if (list && typ != "string" && typ != "[]string") || (!list && typ != definition["type"]) {
				return nil, fmt.Errorf("incompatible enrichment schema %s; no repair performed", k)
			}
			if value, ok := stored["filterable"]; ok && value != nil {
				if _, valid := value.(bool); !valid {
					return nil, fmt.Errorf("invalid filterable setting for %s", k)
				}
			}
			if stored["filterable"] == true {
				continue
			}
			// Never send a native embedding declaration during a filterability repair.
			if _, ok := stored["embed"]; ok {
				return nil, fmt.Errorf("enrichment field %s has embed declaration; no repair performed", k)
			}
			stored["filterable"] = true
			definition = stored
		}
		changes[k] = definition
	}
	return map[string]any{"schema": changes}, nil
}

// ApplySessionEnrichmentFilterPlan applies only the additive schema patch.
func (c *Client) ApplySessionEnrichmentFilterPlan(ctx context.Context) error {
	plan, err := c.SessionEnrichmentFilterPlan(ctx)
	if err != nil {
		return err
	}
	var out writeResponse
	if err = c.doContext(ctx, "POST", "/v2/namespaces/"+c.Namespace+"-sessions", plan, &out); err != nil {
		return err
	}
	if out.Error != "" {
		return fmt.Errorf("schema repair: %s", out.Error)
	}
	return nil
}
