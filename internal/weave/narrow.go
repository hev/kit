package weave

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Narrowing selects part of a Markdown source by heading. It only strips:
// every byte it keeps is a byte of the source, in source order, and nothing is
// rewritten or summarised. Resolution is deterministic and needs no model.
//
// A selector is an ATX heading as written in the file, hashes included:
// "## Testing" selects every level-2 heading whose text is "Testing" (after
// trimming, collapsing inner whitespace and removing a closing run of #).
// "(preamble)" names the text before the first heading. A heading's section
// runs to the next heading of the same or a higher level. Headings inside
// fenced code blocks are text, as are Setext (underlined) headings.
//
// keep lists the sections to keep: the preamble stays, and so do the heading
// lines of each kept section's ancestors, so the result still reads in
// context; everything else goes. drop lists sections to remove, applied after
// keep. A selector that matches no heading is an error, and so is a result
// with nothing left in it.

// Preamble selects the text before a file's first heading.
const Preamble = "(preamble)"

// Selection records how a source was narrowed. It is part of the bundle
// manifest and digest; whole-file sources have none, so their bytes and
// digests are unchanged.
type Selection struct {
	SourceSHA256 string    `json:"source_sha256"`
	SourceBytes  int       `json:"source_bytes"`
	Keep         []string  `json:"keep,omitempty"`
	Drop         []string  `json:"drop,omitempty"`
	Kept         []Section `json:"kept"`
	Dropped      []Section `json:"dropped"`
}

// Section is one heading's own block: its heading line up to the next heading
// of any level. HeadingOnly marks a kept ancestor whose body was dropped.
type Section struct {
	Heading     string `json:"heading"`
	Level       int    `json:"level"`
	Line        int    `json:"line"`
	Lines       int    `json:"lines"`
	Bytes       int    `json:"bytes"`
	HeadingOnly bool   `json:"heading_only,omitempty"`
}

// Outline is one heading in a Markdown file with the size of its whole
// section (subsections included), for planning.
type Outline struct {
	Heading string `json:"heading"`
	Level   int    `json:"level"`
	Line    int    `json:"line"`
	Bytes   int    `json:"bytes"`
}

type heading struct {
	level      int
	text       string // normalized "## Text"
	line       int    // index into lines
	end        int    // section end (exclusive), subsections included
	blockEnd   int    // own block end (exclusive)
	parent     int    // index of the enclosing heading, -1 for none
	sectionLen int
}

var (
	atxRE   = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	fenceRE = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	spaceRE = regexp.MustCompile(`[ \t]+`)
)

// splitLines keeps each line's terminator so joining is lossless.
func splitLines(data []byte) []string {
	s := string(data)
	var out []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

func normalizeHeading(level int, text string) string {
	text = strings.TrimSpace(text)
	// A closing sequence of # is decoration when preceded by a space.
	if t := strings.TrimRight(text, "#"); t != text && (t == "" || strings.HasSuffix(t, " ") || strings.HasSuffix(t, "\t")) {
		text = strings.TrimSpace(t)
	}
	text = spaceRE.ReplaceAllString(text, " ")
	return strings.Repeat("#", level) + " " + text
}

// NormalizeSelector checks a selector and returns its canonical form.
func NormalizeSelector(s string) (string, error) {
	t := strings.TrimSpace(s)
	if t == Preamble {
		return t, nil
	}
	m := atxRE.FindStringSubmatch(t)
	if m == nil || strings.TrimSpace(m[2]) == "" {
		return "", fmt.Errorf("invalid section selector %q: write the heading as it appears, e.g. \"## Testing\", or %q", s, Preamble)
	}
	return normalizeHeading(len(m[1]), m[2]), nil
}

func parseHeadings(lines []string) []heading {
	var hs []heading
	fence := ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r\n")
		if m := fenceRE.FindStringSubmatch(line); m != nil {
			marker := m[1]
			switch {
			case fence == "":
				fence = marker
			case marker[0] == fence[0] && len(marker) >= len(fence) && strings.TrimSpace(line[strings.Index(line, marker)+len(marker):]) == "":
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if m := atxRE.FindStringSubmatch(line); m != nil {
			hs = append(hs, heading{level: len(m[1]), text: normalizeHeading(len(m[1]), m[2]), line: i, parent: -1})
		}
	}
	for i := range hs {
		hs[i].end, hs[i].blockEnd = len(lines), len(lines)
		if i+1 < len(hs) {
			hs[i].blockEnd = hs[i+1].line
		}
		for j := i + 1; j < len(hs); j++ {
			if hs[j].level <= hs[i].level {
				hs[i].end = hs[j].line
				break
			}
		}
		for j := i - 1; j >= 0; j-- {
			if hs[j].level < hs[i].level {
				hs[i].parent = j
				break
			}
		}
	}
	return hs
}

func sum(lines []string, from, to int) int {
	n := 0
	for _, l := range lines[from:to] {
		n += len(l)
	}
	return n
}

// Sections outlines a Markdown file's headings with whole-section sizes.
func Sections(data []byte) []Outline {
	lines := splitLines(data)
	out := []Outline{}
	hs := parseHeadings(lines)
	if len(hs) == 0 || hs[0].line > 0 {
		end := len(lines)
		if len(hs) > 0 {
			end = hs[0].line
		}
		if n := sum(lines, 0, end); n > 0 {
			out = append(out, Outline{Heading: Preamble, Line: 1, Bytes: n})
		}
	}
	for _, h := range hs {
		out = append(out, Outline{Heading: h.text, Level: h.level, Line: h.line + 1, Bytes: sum(lines, h.line, h.end)})
	}
	return out
}

// Narrow applies keep and drop selectors to a Markdown source.
func Narrow(data []byte, keep, drop []string) ([]byte, *Selection, error) {
	if len(keep) == 0 && len(drop) == 0 {
		return nil, nil, errors.New("narrowing needs keep or drop selectors")
	}
	lines := splitLines(data)
	hs := parseHeadings(lines)
	preambleEnd := len(lines)
	if len(hs) > 0 {
		preambleEnd = hs[0].line
	}
	match := func(sel string) ([]int, error) {
		n, err := NormalizeSelector(sel)
		if err != nil {
			return nil, err
		}
		if n == Preamble {
			if preambleEnd == 0 || sum(lines, 0, preambleEnd) == 0 {
				return nil, fmt.Errorf("section selector %q matches nothing: the file starts with a heading", sel)
			}
			return []int{-1}, nil
		}
		var found []int
		for i, h := range hs {
			if h.text == n {
				found = append(found, i)
			}
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("section selector %q matches no heading", sel)
		}
		return found, nil
	}
	kept := make([]bool, len(lines))
	sel := &Selection{SourceSHA256: hash(data), SourceBytes: len(data), Keep: keep, Drop: drop, Kept: []Section{}, Dropped: []Section{}}
	if len(keep) == 0 {
		for i := range kept {
			kept[i] = true
		}
	} else {
		for i := 0; i < preambleEnd; i++ {
			kept[i] = true
		}
		for _, s := range keep {
			found, err := match(s)
			if err != nil {
				return nil, nil, err
			}
			for _, i := range found {
				if i < 0 {
					continue // the preamble is already kept
				}
				for l := hs[i].line; l < hs[i].end; l++ {
					kept[l] = true
				}
				for p := hs[i].parent; p >= 0; p = hs[p].parent {
					kept[hs[p].line] = true
				}
			}
		}
	}
	for _, s := range drop {
		found, err := match(s)
		if err != nil {
			return nil, nil, err
		}
		for _, i := range found {
			from, to := 0, preambleEnd
			if i >= 0 {
				from, to = hs[i].line, hs[i].end
			}
			for l := from; l < to; l++ {
				kept[l] = false
			}
		}
	}
	var out strings.Builder
	for i, l := range lines {
		if kept[i] {
			out.WriteString(l)
		}
	}
	if strings.TrimSpace(out.String()) == "" {
		return nil, nil, errors.New("narrowing leaves nothing; drop the whole source from the recipe instead")
	}
	block := func(name string, level, from, to int) {
		var k, total int
		for l := from; l < to; l++ {
			total += len(lines[l])
			if kept[l] {
				k += len(lines[l])
			}
		}
		if total == 0 {
			return
		}
		s := Section{Heading: name, Level: level, Line: from + 1, Lines: to - from}
		switch {
		case k == total:
			s.Bytes = total
			sel.Kept = append(sel.Kept, s)
		case k == 0:
			s.Bytes = total
			sel.Dropped = append(sel.Dropped, s)
		default:
			// Only a kept ancestor's heading line survives in a mixed block.
			s.Bytes, s.HeadingOnly = k, true
			sel.Kept = append(sel.Kept, s)
			d := s
			d.Bytes, d.HeadingOnly = total-k, false
			d.Heading = name + " (body)"
			sel.Dropped = append(sel.Dropped, d)
		}
	}
	block(Preamble, 0, 0, preambleEnd)
	for _, h := range hs {
		block(h.text, h.level, h.line, h.blockEnd)
	}
	return []byte(out.String()), sel, nil
}
