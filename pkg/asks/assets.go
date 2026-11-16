package asks

import "embed"

// PageTemplate is the shared dashboard template, including the asks view.
//
//go:embed assets/template.html
var PageTemplate string

// hostedPageTemplate uses DOM setters for stored content; the local template
// stays byte-identical for existing local consumers.
//
//go:embed assets/hosted-template.html
var hostedPageTemplate string

// Assets holds the shared dashboard's styles and modules.
//
//go:embed assets/ui
var Assets embed.FS
