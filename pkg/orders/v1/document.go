package orders

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/hev/kit/internal/dispatch"
)

// Destination validates the document using dispatch's own parser.
func Destination(text string) (id, host string, err error) {
	order, err := dispatch.Parse(text)
	if err != nil {
		return "", "", ErrRejected
	}
	return order.Name, order.Host, nil
}

// Document renders an already schema-validated spec as flat frontmatter.
func Document(spec map[string]any, body string) (string, error) {
	keys := make([]string, 0, len(spec))
	for key := range spec {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var doc strings.Builder
	doc.WriteString("---\n")
	for _, key := range keys {
		if strings.ContainsAny(key, "\r\n:#") {
			return "", ErrRejected
		}
		switch value := spec[key].(type) {
		case string:
			if strings.ContainsAny(value, "\r\n") {
				return "", ErrRejected
			}
			quote := "\""
			if strings.Contains(value, quote) {
				quote = "'"
			}
			if strings.Contains(value, quote) {
				return "", ErrRejected
			}
			fmt.Fprintf(&doc, "%s: %s%s%s\n", key, quote, value, quote)
		case json.Number, float64, []any:
			raw, err := json.Marshal(value)
			if err != nil {
				return "", ErrRejected
			}
			fmt.Fprintf(&doc, "%s: %s\n", key, raw)
		default:
			return "", ErrRejected
		}
	}
	doc.WriteString("---\n")
	doc.WriteString(body)
	return doc.String(), nil
}
