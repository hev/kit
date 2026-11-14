package daemon

// KitConfig is this machine's sign-in to a hosted kit service. Key is an
// opaque kit key; the service decides what it may reach. Author fields label
// rows captured here; the service, not this machine, is their authority.
type KitConfig struct {
	Service    string `toml:"service"`
	API        string `toml:"api,omitempty"`
	Key        string `toml:"key,omitempty"`
	KeyID      string `toml:"key_id,omitempty"`
	TeamID     string `toml:"team_id,omitempty"`
	TeamSlug   string `toml:"team_slug,omitempty"`
	AuthorID   string `toml:"author_id,omitempty"`
	AuthorName string `toml:"author_name,omitempty"`
}

// SignedIn reports whether a kit key is configured, as opposed to a [kit]
// table that only names the service to sign in to.
func (k *KitConfig) SignedIn() bool { return k != nil && k.Key != "" }

// KitCaptureNotice is what a daemon signed into kit, with no archive target of
// its own, says instead of failing.
const KitCaptureNotice = "signed in to kit, but hosted capture is not wired yet: sessions stay on this machine and are not uploaded"

// WriteKitConfig records a sign-in and moves the current [layer] target aside
// so ClearKitConfig can restore it. A [kit] table that only named the service
// does not count as a sign-in, so the first real one still saves the target.
func WriteKitConfig(doc map[string]any, kit KitConfig) error {
	if _, saved := doc["kit_prior"]; !saved {
		if previous, ok := doc["layer"]; ok {
			doc["kit_prior"] = previous
		}
	}
	delete(doc, "layer")
	doc["kit"] = kit
	table(doc, "capture")["redact"] = true
	return writeInviteDocument(doc)
}

// ClearKitConfig removes the sign-in and restores the prior target. The
// service URL is kept so the next `hev login` needs no flag.
func ClearKitConfig(doc map[string]any) error {
	service := ""
	switch k := doc["kit"].(type) {
	case map[string]any:
		service, _ = k["service"].(string)
	case KitConfig:
		service = k.Service
	}
	delete(doc, "kit")
	if service != "" {
		doc["kit"] = map[string]any{"service": service}
	}
	delete(doc, "layer")
	if previous, ok := doc["kit_prior"]; ok {
		doc["layer"] = previous
	}
	delete(doc, "kit_prior")
	return writeInviteDocument(doc)
}
