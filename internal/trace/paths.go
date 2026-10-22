package trace

import "strings"

// WindowsPath recognizes drive-qualified and UNC transcript paths on any OS.
func WindowsPath(p string) bool {
	return len(p) >= 3 && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) && p[1] == ':' && (p[2] == '\\' || p[2] == '/') || strings.HasPrefix(p, `\\`)
}

// Claude's encoding is lossy (hyphens can also occur in directory names).
// Prefer cwd from the transcript; this fallback only decodes Windows slugs.
func claudeProjectDir(slug string) string {
	if len(slug) >= 3 && slug[1:3] == "--" && ((slug[0] >= 'A' && slug[0] <= 'Z') || (slug[0] >= 'a' && slug[0] <= 'z')) {
		return slug[:1] + `:\` + strings.ReplaceAll(slug[3:], "-", `\`)
	}
	return slug
}
