// Package dispatch sends bounded agent runs to configured machines.
package dispatch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Order struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Repo       string `json:"repo"`
	Base       string `json:"base"`
	Harness    string `json:"harness"`
	Model      string `json:"model,omitempty"`
	Validation string `json:"validation"`
	Reply      string `json:"reply,omitempty"`
	Issue      string `json:"issue,omitempty"`
	Timeout    int    `json:"timeout"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func ValidName(s string) bool { return namePattern.MatchString(s) }
func Parse(text string) (Order, error) {
	o := Order{Base: "main", Harness: "codex", Validation: "none", Timeout: 120}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return o, fmt.Errorf("order must start with ---")
	}
	fields := map[string]string{}
	end := 0
	for i, line := range lines[1:] {
		if line == "---" {
			end = i + 1
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return o, fmt.Errorf("expected flat key: value")
		}
		v = strings.TrimSpace(v)
		switch k {
		case "name", "host", "repo", "base", "harness", "model", "validation", "reply", "issue", "linear", "timeout":
		default:
			return o, fmt.Errorf("unknown key: %s", k)
		}
		if k == "linear" {
			k = "issue"
		}
		if _, ok := fields[k]; ok {
			return o, fmt.Errorf("duplicate key: %s", k)
		}
		if strings.HasPrefix(v, "\"") || strings.HasPrefix(v, "'") {
			q := v[:1]
			j := strings.Index(v[1:], q)
			if j < 0 {
				return o, fmt.Errorf("unclosed quote")
			}
			rest := strings.TrimSpace(v[j+2:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return o, fmt.Errorf("invalid scalar")
			}
			v = v[1 : j+1]
		} else {
			v = strings.TrimSpace(strings.SplitN(v, "#", 2)[0])
			if strings.ContainsAny(v, "[]{}\t") {
				return o, fmt.Errorf("expected scalar")
			}
		}
		if v == "" {
			return o, fmt.Errorf("empty field: %s", k)
		}
		fields[k] = v
	}
	if end == 0 {
		return o, fmt.Errorf("unclosed frontmatter")
	}
	accept := false
	for _, l := range lines[end+1:] {
		if strings.TrimSpace(l) == "## Acceptance" {
			accept = true
		}
	}
	if !accept {
		return o, fmt.Errorf("missing ## Acceptance section")
	}
	for k, v := range fields {
		switch k {
		case "name":
			o.Name = v
		case "host":
			o.Host = v
		case "repo":
			o.Repo = v
		case "base":
			o.Base = v
		case "harness":
			o.Harness = v
		case "model":
			o.Model = v
		case "validation":
			o.Validation = v
		case "reply":
			o.Reply = v
		case "issue":
			o.Issue = v
		case "timeout":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 525600 {
				return o, fmt.Errorf("invalid timeout")
			}
			o.Timeout = n
		}
	}
	if !ValidName(o.Name) {
		return o, fmt.Errorf("invalid name")
	}
	if o.Repo == "" {
		return o, fmt.Errorf("repo required")
	}
	if o.Harness != "codex" && o.Harness != "claude" {
		return o, fmt.Errorf("invalid harness")
	}
	if o.Validation != "none" && o.Validation != "browser" {
		return o, fmt.Errorf("invalid validation")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9/_.-]*$`).MatchString(o.Base) || strings.Contains(o.Base, "..") {
		return o, fmt.Errorf("invalid base")
	}
	return o, nil
}
func CheckCap(n, cap int) error {
	if n >= cap {
		return fmt.Errorf("host full (%d/%d)", n, cap)
	}
	return nil
}
