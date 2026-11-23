// Package kit embeds repository-level data that internal packages share.
package kit

import _ "embed"

// PricesTOML is prices.toml, the single source of verified list rates. Trace
// pricing reads it from the binary so a dashboard never depends on the
// working directory.
//
//go:embed prices.toml
var PricesTOML []byte
