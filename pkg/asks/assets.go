package asks

import "embed"

// PageTemplate is the shared dashboard template, including the asks view.
//
//go:embed assets/template.html
var PageTemplate string

// Assets holds the shared dashboard's styles and modules.
//
//go:embed assets/ui
var Assets embed.FS
