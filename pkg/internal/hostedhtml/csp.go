// Package hostedhtml supplies document policy for authenticated views.
package hostedhtml

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
)

var scripts = regexp.MustCompile(`(?s)<script(?:\s[^>]*)?>(.*?)</script>`)

// Policy authorizes only the inline script bytes in this rendered document.
// Same-origin modules and inline styles are required by the existing UI.
// Event-handler attributes and embedded plugins are never authorized.
func Policy(document string) string {
	sources := []string{"'self'"}
	for _, m := range scripts.FindAllStringSubmatch(document, -1) {
		sum := sha256.Sum256([]byte(m[1]))
		sources = append(sources, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return "default-src 'self'; script-src " + strings.Join(sources, " ") + "; script-src-attr 'none'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'"
}
