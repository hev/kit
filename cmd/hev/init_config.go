package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pelletier/go-toml/v2/unstable"
)

func loadInitConfig(path string) (initFileConfig, error) {
	var cfg initFileConfig
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if !info.Mode().IsRegular() {
		return cfg, fmt.Errorf("config %s must be a regular file (not a symlink)", path)
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	// Validate the whole document, including duplicate definitions. Decode only
	// the prompted fields: capture and future settings must not be normalized.
	var doc map[string]any
	if _, err := toml.Decode(string(raw), &doc); err != nil {
		return cfg, fmt.Errorf("read config %s: invalid TOML", path)
	}
	var target struct {
		Layer initLayerConfig `toml:"layer"`
	}
	if _, err := toml.Decode(string(raw), &target); err != nil {
		return cfg, fmt.Errorf("read config %s: layer target fields must be strings", path)
	}
	cfg.Layer = target.Layer
	cfg.source, cfg.exists = raw, true
	_, cfg.hasLocal = doc["local"]
	return cfg, nil
}

type initConfigEdit struct {
	start, end int
	text       string
}

// patchInitConfig uses syntax ranges, not line matching or re-encoding: quoted
// keys and multiline values are parsed, while every other source byte survives.
// The unstable parser API is isolated here and pinned in go.mod.
func patchInitConfig(raw []byte, target initLayerConfig) ([]byte, error) {
	fields := []string{"endpoint", "api_key", "namespace"}
	values := []string{target.Endpoint, target.APIKey, target.Namespace}
	encoded := make(map[string]string)
	requested := make(map[string]string)
	for i, key := range fields {
		requested[key] = values[i]
		var b bytes.Buffer
		if err := toml.NewEncoder(&b).Encode(map[string]string{key: values[i]}); err != nil {
			return nil, err
		}
		encoded[key] = strings.TrimSpace(strings.SplitN(b.String(), "=", 2)[1])
	}
	var edits []initConfigEdit
	found := make(map[string]bool)
	tableInsert, inlineInsert := -1, -1
	inlineNonempty := false
	var table []string
	var parser unstable.Parser
	parser.Reset(raw)
	keys := func(n *unstable.Node) []string {
		var result []string
		it := n.Key()
		for it.Next() {
			result = append(result, string(it.Node().Data))
		}
		return result
	}
	var visit func(*unstable.Node, []string)
	visit = func(n *unstable.Node, prefix []string) {
		path := append(append([]string(nil), prefix...), keys(n)...)
		value := n.Value()
		if len(path) == 2 && path[0] == "layer" {
			if replacement, ok := encoded[path[1]]; ok && value.Kind == unstable.String {
				found[path[1]] = true
				// Keeping an unchanged value also keeps its quoting and escapes.
				if string(value.Data) != requested[path[1]] {
					start := int(value.Raw.Offset)
					edits = append(edits, initConfigEdit{start, start + int(value.Raw.Length), replacement})
				}
			}
		}
		if value.Kind == unstable.InlineTable {
			if len(path) == 1 && path[0] == "layer" {
				inlineInsert = int(value.Raw.Offset) + 1
				inlineNonempty = value.Child() != nil
			}
			it := value.Children()
			for it.Next() {
				visit(it.Node(), path)
			}
		}
	}
	for parser.NextExpression() {
		n := parser.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = keys(n)
			if n.Kind == unstable.Table && len(table) == 1 && table[0] == "layer" {
				// The header key is a single-line token, even if it contains an escaped
				// newline. Insert after the entire header and its trailing comment.
				start := int(n.Child().Raw.Offset)
				if end := bytes.IndexByte(raw[start:], '\n'); end >= 0 {
					tableInsert = start + end + 1
				} else {
					tableInsert = len(raw)
				}
			}
		case unstable.KeyValue:
			visit(n, table)
		}
	}
	if parser.Error() != nil {
		return nil, fmt.Errorf("cannot preserve config: unsupported TOML syntax")
	}
	newline := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		newline = "\r\n"
	}
	var missing []string
	for _, key := range fields {
		if !found[key] {
			missing = append(missing, key+" = "+encoded[key])
		}
	}
	if len(missing) > 0 {
		switch {
		case inlineInsert >= 0:
			text := strings.Join(missing, ", ")
			if inlineNonempty {
				text += ", "
			}
			edits = append(edits, initConfigEdit{inlineInsert, inlineInsert, text})
		case tableInsert >= 0:
			text := strings.Join(missing, newline) + newline
			if tableInsert > 0 && raw[tableInsert-1] != '\n' {
				text = newline + text
			}
			edits = append(edits, initConfigEdit{tableInsert, tableInsert, text})
		default:
			// Root dotted keys extend a missing or implicitly defined layer table.
			text := ""
			for _, field := range missing {
				text += "layer." + field + newline
			}
			edits = append(edits, initConfigEdit{0, 0, text})
		}
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var result bytes.Buffer
	pos := 0
	for _, edit := range edits {
		result.Write(raw[pos:edit.start])
		result.WriteString(edit.text)
		pos = edit.end
	}
	result.Write(raw[pos:])
	// Fail closed if an unexpected syntax shape cannot be extended safely.
	var check struct {
		Layer initLayerConfig `toml:"layer"`
	}
	if _, err := toml.Decode(result.String(), &check); err != nil || check.Layer != target {
		return nil, fmt.Errorf("cannot preserve config while updating layer target")
	}
	return result.Bytes(), nil
}

func writeInitConfig(path string, cfg initFileConfig) error {
	var raw []byte
	var err error
	if cfg.exists {
		raw, err = patchInitConfig(cfg.source, cfg.Layer)
	} else {
		var buf bytes.Buffer
		err = toml.NewEncoder(&buf).Encode(cfg)
		raw = buf.Bytes()
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Never truncate the existing config. Rename a private, complete file from
	// the same directory, and remove it on all failure paths.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".init-config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
