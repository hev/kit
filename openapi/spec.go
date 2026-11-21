// Package openapi embeds kit's versioned API contract.
package openapi

import _ "embed"

// KitV1 is the OpenAPI 3.1 contract served at /openapi/kit-v1.yaml.
//
//go:embed kit-v1.yaml
var KitV1 string
