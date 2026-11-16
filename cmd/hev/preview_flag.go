package main

import "github.com/spf13/pflag"

// previewFlag registers --preview, the name for "show what would happen and
// change nothing", with --dry-run kept as a hidden alias for older scripts.
func previewFlag(fs *pflag.FlagSet, p *bool, usage string) {
	fs.BoolVar(p, "preview", false, usage)
	fs.BoolVar(p, "dry-run", false, usage+" (alias of --preview)")
	_ = fs.MarkHidden("dry-run")
}
