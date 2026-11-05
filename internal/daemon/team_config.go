package daemon

// TeamConfig stores this machine's membership, never the browser session.
type TeamConfig struct {
	Service string `toml:"service"`
	KeyID   string `toml:"key_id"`
	Author  struct {
		ID    string `json:"id" toml:"id"`
		Name  string `json:"name" toml:"name"`
		Email string `json:"email" toml:"email"`
	} `json:"author" toml:"author"`
}

func WriteTeamConfig(doc map[string]any, endpoint, namespace, key string, team TeamConfig) error {
	if doc["team"] == nil {
		if previous, ok := doc["layer"]; ok {
			doc["team_local"] = previous
		}
	}
	doc["team"] = team
	doc["layer"] = map[string]any{"endpoint": endpoint, "namespace": namespace, "api_key": key}
	table(doc, "capture")["redact"] = true
	return writeInviteDocument(doc)
}
func ClearTeamConfig(doc map[string]any) error {
	delete(doc, "layer")
	if previous, ok := doc["team_local"]; ok {
		doc["layer"] = previous
	}
	delete(doc, "team_local")
	delete(doc, "team")
	return writeInviteDocument(doc)
}
