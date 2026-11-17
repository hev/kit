package weave

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"path/filepath"
	"strings"
)

// A plan is the mechanical starting point for a context review: every
// instruction file, agent and skill the assessment discovers, with its size,
// sections and findings, and a starting recipe that keeps all of it whole.
// Choosing what to cut is the reviewer's job; nothing here calls a model.

// PlanFile is one discovered source.
type PlanFile struct {
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	Bytes    int       `json:"bytes"`
	SHA256   string    `json:"sha256"`
	Sections []Outline `json:"sections"`
	Findings []Finding `json:"findings"`
	Text     string    `json:"-"`
}

// Plan is plan.json.
type Plan struct {
	Version    int        `json:"version"`
	Task       string     `json:"task"`
	Bytes      int        `json:"bytes"`
	Files      []PlanFile `json:"files"`
	Assessment Assessment `json:"assessment"`
	Recipe     Recipe     `json:"recipe"`
}

// SourceKind names what a discovered path is to a harness.
func SourceKind(p string) string {
	switch {
	case filepath.Base(p) == "AGENTS.md" || filepath.Base(p) == "CLAUDE.md":
		return "instruction"
	case strings.HasPrefix(p, ".claude/agents/"):
		return "agent"
	default:
		return "skill"
	}
}

// MakePlan discovers root's sources and proposes the whole-file recipe.
func MakePlan(root, task string) (Plan, error) {
	if strings.TrimSpace(task) == "" {
		return Plan{}, errors.New("the order has no assignment")
	}
	r, err := Discover(root)
	if err != nil {
		return Plan{}, err
	}
	r.Task = task
	a := Assess(root, r)
	p := Plan{Version: 1, Task: task, Files: []PlanFile{}, Assessment: a, Recipe: r}
	for _, s := range r.Sources {
		data, err := read(root, s.Path)
		if err != nil {
			return p, err
		}
		f := PlanFile{Path: s.Path, Kind: SourceKind(s.Path), Bytes: len(data), SHA256: hash(data), Sections: []Outline{}, Findings: []Finding{}, Text: string(data)}
		if strings.HasSuffix(s.Path, ".md") {
			f.Sections = Sections(data)
		}
		for _, fd := range a.Findings {
			if fd.Path == s.Path {
				f.Findings = append(f.Findings, fd)
			}
		}
		p.Bytes += len(data)
		p.Files = append(p.Files, f)
	}
	return p, nil
}

// PublishPlan writes plan.json, recipe.json and index.html to a new directory.
func PublishPlan(out string, p Plan) error {
	page, err := renderPlan(p)
	if err != nil {
		return err
	}
	return publish(out, map[string][]byte{"plan.json": encode(p), "recipe.json": encode(p.Recipe), "index.html": page})
}

// Summary prints a plan for a terminal: each file with its size and its
// top-level sections, then the findings count.
func (p Plan) Summary(w io.Writer) {
	fmt.Fprintf(w, "%d sources, %d bytes, %d findings\n", len(p.Files), p.Bytes, len(p.Assessment.Findings))
	for _, f := range p.Files {
		fmt.Fprintf(w, "  %-11s %8d  %s\n", f.Kind, f.Bytes, f.Path)
		for _, s := range f.Sections {
			if s.Level <= 2 {
				fmt.Fprintf(w, "  %11s %8d    %s\n", "", s.Bytes, s.Heading)
			}
		}
		for _, fd := range f.Findings {
			fmt.Fprintf(w, "  %11s %8s    ! %s:%d %s\n", "", "", fd.Rule, fd.Line, fd.Detail)
		}
	}
}

var planPage = template.Must(template.New("plan").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>Context plan</title><style>
:root{color-scheme:light dark}body{margin:0 auto;max-width:1100px;padding:24px;font:15px/1.55 system-ui,sans-serif}h1{font-size:26px}code,pre{font:13px/1.6 ui-monospace,monospace}pre{white-space:pre-wrap;overflow-wrap:anywhere;border-left:3px solid #8884;padding-left:12px}table{border-collapse:collapse;width:100%}td,th{text-align:left;padding:3px 8px;border-bottom:1px solid #8883}td.n{text-align:right;font-variant-numeric:tabular-nums}.f{color:#b45309}details{margin:6px 0}
</style></head><body>
<h1>Context plan</h1>
<p>{{len .Files}} sources, {{.Bytes}} bytes, {{len .Assessment.Findings}} findings. The starting recipe keeps every file whole: decide what this task needs, then keep or drop sections by heading (<code>"keep": ["## Heading"]</code> or <code>"drop": [...]</code>) or remove a source.</p>
<h2>Assignment</h2><pre>{{.Task}}</pre>
<h2>Sources</h2>
<table><tr><th>Source</th><th>Kind</th><th>Bytes</th><th>Sections</th><th>Findings</th></tr>
{{range .Files}}<tr><td><a href="#{{.Path}}">{{.Path}}</a></td><td>{{.Kind}}</td><td class="n">{{.Bytes}}</td><td class="n">{{len .Sections}}</td><td class="n">{{len .Findings}}</td></tr>
{{end}}</table>
{{range .Files}}<h2 id="{{.Path}}">{{.Path}}</h2><p>{{.Kind}} · {{.Bytes}} bytes · SHA-256 <code>{{.SHA256}}</code></p>
{{if .Sections}}<table><tr><th>Line</th><th>Section (selector)</th><th>Bytes with subsections</th></tr>{{range .Sections}}<tr><td class="n">{{.Line}}</td><td><code>{{.Heading}}</code></td><td class="n">{{.Bytes}}</td></tr>{{end}}</table>{{end}}
{{range .Findings}}<p class="f">{{.Rule}} · line {{.Line}} · {{.Detail}}</p>{{end}}
<details><summary>Full text</summary><pre>{{.Text}}</pre></details>
{{end}}
<h2>Starting recipe</h2><pre>{{.RecipeJSON}}</pre>
<p><small>Findings are deterministic lexical checks, not semantic analysis. Sources are shown as text, never rendered.</small></p>
</body></html>
`))

func renderPlan(p Plan) ([]byte, error) {
	var b bytes.Buffer
	err := planPage.Execute(&b, struct {
		Plan
		RecipeJSON string
	}{p, string(encode(p.Recipe))})
	return b.Bytes(), err
}
