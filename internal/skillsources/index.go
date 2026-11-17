package skillsources

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Chunk is one searchable piece of a pinned pack. Its identity is (source,
// commit, path, part), so a bump writes new rows instead of rewriting the
// ones a reviewed pin produced; every chunk carries its licence and
// attribution.
type Chunk struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Pack     string `json:"pack"`
	Skill    string `json:"skill"`
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	URL      string `json:"url"`
	Commit   string `json:"commit"`
	Path     string `json:"path"`
	Part     int    `json:"part"`
	License  string `json:"license"`
	Author   string `json:"author"`
	Homepage string `json:"homepage"`
}

// ChunkKey is the row identity for one part of one file at one pin.
func ChunkKey(source, commit, path string, part int) string {
	return "skill-" + hash([]byte(source + "\x00" + commit + "\x00" + path + "\x00" + strconv.Itoa(part)))[:32]
}

// chunkRunes keeps a chunk inside the embedding model's comfortable input.
const chunkRunes = 2000

// Chunks reads a pack's pinned skills: every Markdown and text file inside
// each skill directory, split on paragraph boundaries.
func Chunks(p Pack) ([]Chunk, error) {
	var out []Chunk
	for _, sk := range p.Skills {
		dir := filepath.Join(p.Pin, filepath.FromSlash(sk.Path))
		var files []string
		err := filepath.WalkDir(dir, func(f string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if f != dir && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(f)) {
			case ".md", ".txt", ".mdx":
				files = append(files, f)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			if !utf8.Valid(b) || licenseFile(filepath.Base(f)) {
				continue
			}
			rel, _ := filepath.Rel(p.Pin, f)
			rel = filepath.ToSlash(rel)
			for i, text := range split(string(b), chunkRunes) {
				out = append(out, Chunk{
					ID: ChunkKey(p.Source, p.Commit, rel, i), Text: text,
					Pack: p.Name, Skill: sk.Name, Kind: p.Kind, Source: p.Source, URL: p.URL,
					Commit: p.Commit, Path: rel, Part: i, License: sk.License, Author: p.Author, Homepage: p.Home,
				})
			}
		}
	}
	return out, nil
}

// split cuts text at blank lines into pieces of at most max runes; a single
// longer paragraph is cut by runes.
func split(text string, max int) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, para := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		for utf8.RuneCountInString(para) > max {
			flush()
			r := []rune(para)
			out = append(out, strings.TrimSpace(string(r[:max])))
			para = string(r[max:])
		}
		if utf8.RuneCountInString(cur.String())+utf8.RuneCountInString(para)+2 > max {
			flush()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(para)
	}
	flush()
	return out
}
