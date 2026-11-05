package peers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// relayAlias uses the existing beacon snapshot schema. Local names win; cached
// names on several hosts require qualification rather than directory-order luck.
func (e *Engine) relayAlias(target string, local []Peer) (string, error) {
	if strings.ContainsAny(target, "/:") || strings.HasPrefix(target, "%") || os.Getenv("PEERS_REMOTE_FROM") != "" {
		return target, nil
	}
	for _, p := range local {
		if p.Session == target {
			return target, nil
		}
	}
	paths, err := filepath.Glob(filepath.Join(e.State, "hosts", "*.json"))
	if err != nil {
		return "", err
	}
	hosts := map[string]bool{}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snapshot struct {
			Host  string `json:"host"`
			Panes []struct {
				Address string `json:"address"`
				Role    string `json:"role"`
			} `json:"panes"`
		}
		if json.Unmarshal(b, &snapshot) != nil || snapshot.Host == "" || snapshot.Host == e.Host {
			continue
		}
		for _, p := range snapshot.Panes {
			if p.Address == target && (p.Role == "" || p.Role == "foh" || p.Role == "boh") {
				hosts[snapshot.Host] = true
			}
		}
	}
	names := make([]string, 0, len(hosts))
	for host := range hosts {
		names = append(names, host+"/"+target)
	}
	sort.Strings(names)
	if len(names) > 1 {
		return "", fmt.Errorf("%q is on several machines; pick one: %s", target, strings.Join(names, ", "))
	}
	if len(names) == 1 {
		return names[0], nil
	}
	return target, nil
}
