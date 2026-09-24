// Package skills embeds the agent skills that `hev up` installs for Claude Code
// and Codex, so a brew install carries them without a clone.
package skills

import "embed"

// FS holds one directory per skill, each with a SKILL.md at its root.
//
//go:embed hev-query
var FS embed.FS
