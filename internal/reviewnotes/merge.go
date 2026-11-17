package reviewnotes

import (
	"fmt"
	"strings"
)

// Part is one run's notes going into a merge.
type Part struct {
	// Run is the dispatch run that wrote the notes.
	Run string
	// Review marks an independent review run's notes.
	Review bool
	// Head is the commit the notes describe: Notes.Head, or for a review the
	// head named in its report. Empty when unknown.
	Head  string
	Notes Notes
}

// SameHead reports whether two commit names agree, allowing either side to
// be an abbreviation of at least seven characters.
func SameHead(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if len(a) < 7 || len(b) < 7 {
		return a == b && a != ""
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return strings.HasPrefix(b, a)
}

// Merge combines the build run's notes with review runs' notes for head.
//
// Parts keep their order: pass the build run first, then reviews. Files keep
// first-seen order, so the build's files lead and a review's new files
// follow; a review's annotations on a file the build described are appended
// after the build's. Each annotation keeps its author and source; empty ones
// are filled with the part's run name. A part whose head differs from head
// is dropped with a warning. A review part with no head is kept with a
// warning, since its findings cannot be pinned.
func Merge(head string, parts []Part) (Notes, []string) {
	out := Notes{Version: Version, Head: head, Files: []File{}}
	var warnings []string
	index := map[string]int{}
	var summaries []string
	for _, p := range parts {
		switch {
		case p.Head != "" && head != "" && !SameHead(p.Head, head):
			warnings = append(warnings, fmt.Sprintf("dropped %s notes from %s: written for %s, branch head is %s", kind(p), p.Run, Short(p.Head), Short(head)))
			continue
		case p.Head == "" && p.Review:
			warnings = append(warnings, fmt.Sprintf("review notes from %s name no head; kept unverified", p.Run))
		}
		if s := strings.TrimSpace(p.Notes.Summary); s != "" {
			if p.Review {
				s = p.Run + ": " + s
			}
			summaries = append(summaries, s)
		}
		for _, f := range p.Notes.Files {
			i, ok := index[f.Path]
			if !ok {
				i = len(out.Files)
				index[f.Path] = i
				out.Files = append(out.Files, File{Path: f.Path})
			}
			if out.Files[i].Summary == "" {
				out.Files[i].Summary = f.Summary
			}
			for _, a := range f.Annotations {
				if a.Source == "" {
					a.Source = p.Run
				}
				if a.Author == "" {
					a.Author = p.Run
				}
				out.Files[i].Annotations = append(out.Files[i].Annotations, a)
			}
		}
	}
	out.Summary = strings.Join(summaries, "\n\n")
	return out, warnings
}

func kind(p Part) string {
	if p.Review {
		return "review"
	}
	return "build"
}
