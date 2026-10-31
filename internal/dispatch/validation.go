package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ValidatorPrompt deliberately supersedes the implementation worker defaults.
const ValidatorPrompt = `Read ../order.md in full. You are the browser acceptance validator, not an implementer. Never edit the implementation, commit, push, or open a PR. Check every Acceptance item in a real browser, using Playwright or the browser tooling available on this host. If the order has preview, use its URL or run its command in this worktree, replacing {port} with an available local port. Stop the preview process you started when finished. Save screenshots in ../evidence/. Write ../verdict.json as {"verdict":"pass" or "fail","items":[{"item":"exact Acceptance item text","result":"pass" or "fail","evidence":["../evidence/screenshot.png"],"note":"observed behavior"}]}. Include every Acceptance item, with actual evidence and observations, not opinions. A missing browser prerequisite is a failure: report exactly what is missing. Write ../report.md and keep ../status.md current. Do not fix failures.`

type VerdictItem struct {
	Item     string   `json:"item"`
	Result   string   `json:"result"`
	Evidence []string `json:"evidence"`
	Note     string   `json:"note"`
}
type Verdict struct {
	Verdict string        `json:"verdict"`
	Items   []VerdictItem `json:"items"`
}
type ValidationRecord struct {
	Revision string    `json:"revision,omitempty"`
	Expires  time.Time `json:"expires,omitempty"`
	Reset    bool      `json:"reset,omitempty"`
	Error    string    `json:"error,omitempty"`
	Name     string    `json:"name"`
	Host     string    `json:"host"`
	State    string    `json:"state"`
	Evidence string    `json:"evidence,omitempty"`
	Verdict  *Verdict  `json:"verdict,omitempty"`
}

func ParseVerdict(b []byte) (*Verdict, error) {
	var v Verdict
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing verdict data")
	}
	if (v.Verdict != "pass" && v.Verdict != "fail") || len(v.Items) == 0 {
		return nil, fmt.Errorf("invalid verdict")
	}
	failed := false
	for _, i := range v.Items {
		if strings.TrimSpace(i.Item) == "" || (i.Result != "pass" && i.Result != "fail") || strings.TrimSpace(i.Note) == "" {
			return nil, fmt.Errorf("invalid verdict item")
		}
		if i.Result == "fail" {
			failed = true
		}
		if i.Result == "pass" && len(i.Evidence) == 0 {
			return nil, fmt.Errorf("passing item requires evidence")
		}
	}
	if failed != (v.Verdict == "fail") {
		return nil, fmt.Errorf("verdict disagrees with items")
	}
	return &v, nil
}
func CheckValidator(o Order, v ValidationConfig, allow bool) error {
	if v.Harness != "codex" && v.Harness != "claude" {
		return fmt.Errorf("invalid validation harness %q", v.Harness)
	}
	if !allow && o.Harness == v.Harness && o.Model == v.Model {
		return fmt.Errorf("validator %s/%s equals implementer %s/%s; use --allow-same-model", v.Harness, v.Model, o.Harness, o.Model)
	}
	return nil
}
func ShouldValidate(m Meta, r Result, c Config) bool {
	return m.Parent == "" && m.Validation == "browser" && State(&r, false) == "done" && c.Validation.Host != ""
}
func TwiceFailed(records []ValidationRecord) bool {
	counts := map[string]int{}
	for _, r := range records {
		if r.Reset {
			counts = map[string]int{}
		}
		if r.State == "stale" || r.Verdict == nil {
			continue
		}
		seen := map[string]bool{}
		for _, i := range r.Verdict.Items {
			key := strings.Join(strings.Fields(i.Item), " ")
			if i.Result == "fail" && !seen[key] {
				counts[key]++
				seen[key] = true
			}
		}
	}
	for _, n := range counts {
		if n >= 2 {
			return true
		}
	}
	return false
}
func validationRecords(dir string) []ValidationRecord {
	var v []ValidationRecord
	_ = readJSON(filepath.Join(dir, "validation.json"), &v)
	return v
}

// Validate runs on either a caller or the implementer's supervisor host.
func Validate(ctx context.Context, c Config, name, target string, allow bool) (Response, error) {
	return validateFrom(ctx, c, name, target, allow, "")
}
func validateFrom(ctx context.Context, c Config, name, target string, allow bool, source string) (Response, error) {
	var empty Response
	if target == "" {
		target = c.Validation.Host
	}
	h, ok := c.Hosts[target]
	if !ok {
		return empty, fmt.Errorf("unknown validation host %s", target)
	}
	if h.OS == "windows" {
		return empty, fmt.Errorf("Windows validation hosts are unsupported")
	}
	if source == "" {
		for n, r := range Inventory(c, "") {
			for _, row := range r.Rows {
				if row.Name == name {
					if source != "" {
						return empty, fmt.Errorf("ambiguous order")
					}
					source = n
				}
			}
		}
	}
	if source == "" {
		return empty, fmt.Errorf("order not found")
	}
	parent, ok := c.Hosts[source]
	if !ok {
		return empty, fmt.Errorf("unknown parent host %s", source)
	}
	if !(parent.Local && h.Local) && (source == target || (parent.SSH != "" && parent.SSH == h.SSH)) {
		return empty, fmt.Errorf("validation requires a separate host")
	}
	show, err := RPC(ctx, parent, Request{Action: "show", Name: name})
	if err != nil {
		return empty, err
	}
	var m Meta
	var result Result
	if err = json.Unmarshal([]byte(show.Files["meta.json"]), &m); err != nil {
		return empty, err
	}
	if m.Parent != "" || m.Validation != "browser" {
		return empty, fmt.Errorf("validation requires a browser implementation order")
	}
	if err = json.Unmarshal([]byte(show.Files["result.json"]), &result); err != nil || (State(&result, false) != "done" && State(&result, false) != "needs-review") {
		return empty, fmt.Errorf("validation requires a done run")
	}
	if err = CheckValidator(m.Order, c.Validation, allow); err != nil {
		return empty, err
	}
	reservation, err := RPC(ctx, parent, Request{Action: "validation-reserve", Name: name})
	if err != nil {
		return empty, err
	}
	child := reservation.Message
	record := ValidationRecord{Name: child, Host: target, State: "pending", Revision: result.Head, Expires: time.Now().Add(time.Duration(m.Timeout+10) * time.Minute)}
	save := func() {
		_, _ = RPC(ctx, parent, Request{Action: "validation-result", Name: name, ValidationRecord: record})
	}
	vm := Meta{ParentHostName: source, Order: m.Order, Parent: name, ParentHost: parent, Revision: result.Head, ConfigPath: h.ConfigPath, Namespace: h.Namespace}
	if h.Local && vm.ConfigPath == "" {
		vm.ConfigPath = m.ConfigPath
	}
	vm.ParentHost.Env = nil
	vm.Name = child
	vm.Host = target
	vm.Harness = c.Validation.Harness
	vm.Model = c.Validation.Model
	vm.Validation = "none"
	vm.Branch = result.Branch
	q := Request{Action: "start", Name: child, Order: show.Files["order.md"], Meta: vm, Cap: h.Cap, Workspace: c.Workspace}
	response, err := RPC(ctx, h, q)
	if err != nil {
		if !strings.Contains(err.Error(), "host full") && !strings.Contains(err.Error(), "outcome is unknown") {
			record.State = "fail"
		}
		save()
		return empty, err
	}
	record.State = "running"
	save()
	return response, nil
}
func recordValidation(dir string, r ValidationRecord) error {
	var current Result
	_ = readJSON(filepath.Join(dir, "result.json"), &current)
	if r.Revision != "" && r.Revision != current.Head {
		r.State = "stale"
	}
	records := validationRecords(dir)
	found := false
	for i := range records {
		if records[i].Name == r.Name {
			if r.Revision == "" {
				r.Revision = records[i].Revision
			}
			if r.Expires.IsZero() {
				r.Expires = records[i].Expires
			}
			if r.Revision != "" && r.Revision != current.Head {
				r.State = "stale"
			}
			if (records[i].State == "pass" || records[i].State == "fail" || records[i].State == "stale") && (r.State == "running" || r.State == "pending") {
				return nil
			}
			if records[i].State == "running" && r.State == "pending" {
				return nil
			}
			records[i] = r
			found = true
		}
	}
	if !found {
		records = append(records, r)
	}
	if err := WriteJSON(filepath.Join(dir, "validation.json"), records); err != nil {
		return err
	}
	if r.State == "stale" {
		return nil
	}
	var result Result
	if readJSON(filepath.Join(dir, "result.json"), &result) == nil {
		result.Validation = r.State
		result.Verdict = r.Verdict
		result.NeedsReview = TwiceFailed(records)
		return WriteJSON(filepath.Join(dir, "result.json"), result)
	}
	return nil
}
func validationNote(dir string) (string, error) {
	records := validationRecords(dir)
	if len(records) == 0 || records[len(records)-1].State == "stale" || records[len(records)-1].Verdict == nil || records[len(records)-1].Verdict.Verdict != "fail" {
		return "", fmt.Errorf("latest validation has no failed verdict")
	}
	if TwiceFailed(records) {
		return "", fmt.Errorf("needs-review: same Acceptance item failed twice")
	}
	b, _ := json.MarshalIndent(records[len(records)-1].Verdict.Items, "", "  ")
	return "Address the failed browser validation items, preserving Acceptance requirements:\n\n" + string(b), nil
}

// reserveValidation serializes numbering under the parent admission lock.
func reserveValidation(dir, name string) (string, error) {
	records := validationRecords(dir)
	if len(records) > 0 {
		latest := records[len(records)-1]
		// Old records predate leases. Bound them using the last file update.
		if latest.Expires.IsZero() {
			if info, err := os.Stat(filepath.Join(dir, "validation.json")); err == nil {
				var m Meta
				_ = readJSON(filepath.Join(dir, "meta.json"), &m)
				latest.Expires = info.ModTime().Add(time.Duration(m.Timeout+10) * time.Minute)
			}
		}
		if (latest.State == "running" || latest.State == "pending") && !latest.Expires.IsZero() && time.Now().After(latest.Expires) {
			latest.State, latest.Error = "fail", "validation reservation expired; validator did not report a terminal result"
			if err := recordValidation(dir, latest); err != nil {
				return "", err
			}
		}
		if latest.State == "running" {
			return "", fmt.Errorf("validation already running")
		}
		if latest.State == "pending" {
			return latest.Name, nil
		}
	}
	number := 1
	for _, record := range records {
		if !record.Reset {
			number++
		}
	}
	child := fmt.Sprintf("%s-v%d", name, number)
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		return "", err
	}
	return child, recordValidation(dir, ValidationRecord{Name: child, State: "pending", Revision: currentHead(dir), Expires: time.Now().Add(10 * time.Minute)})
}

// VerifyVerdict binds the observations to all original Acceptance items and
// requires referenced artifacts to exist inside this run's evidence directory.
func VerifyVerdict(dir string, v *Verdict) error {
	b, err := os.ReadFile(filepath.Join(dir, "order.md"))
	if err != nil {
		return err
	}
	items := acceptanceItems(string(b))
	if len(items) == 0 {
		return fmt.Errorf("no Acceptance items")
	}
	seen := map[string]bool{}
	for _, item := range v.Items {
		key := strings.Join(strings.Fields(item.Item), " ")
		if !items[key] || seen[key] {
			return fmt.Errorf("unknown or duplicate Acceptance item %q", item.Item)
		}
		seen[key] = true
		for _, path := range item.Evidence {
			full := path
			if !filepath.IsAbs(full) {
				full = filepath.Join(dir, "wt", path)
			}
			root, rootErr := filepath.EvalSymlinks(filepath.Join(dir, "evidence"))
			if rootErr != nil {
				return rootErr
			}
			resolved, err := filepath.EvalSymlinks(full)
			if err != nil {
				return fmt.Errorf("evidence %q: %w", path, err)
			}
			rel, err := filepath.Rel(root, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("evidence outside run directory")
			}
			info, err := os.Stat(resolved)
			if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
				return fmt.Errorf("missing or empty evidence %q", path)
			}
		}
	}
	if len(seen) != len(items) {
		return fmt.Errorf("verdict omits Acceptance items")
	}
	return nil
}

var acceptanceBullet = regexp.MustCompile(`^(?:[-*+]|[0-9]+[.)])\s+`)

func acceptanceItems(order string) map[string]bool {
	result := map[string]bool{}
	active := false
	current := ""
	flush := func() {
		if current != "" {
			result[strings.Join(strings.Fields(current), " ")] = true
			current = ""
		}
	}
	for _, line := range strings.Split(order, "\n") {
		text := strings.TrimSpace(line)
		if text == "## Acceptance" {
			active = true
			continue
		}
		if !active {
			continue
		}
		if strings.HasPrefix(text, "## ") {
			break
		}
		if text == "" {
			continue
		}
		if marker := acceptanceBullet.FindString(text); marker != "" {
			flush()
			current = text[len(marker):]
		} else {
			if current != "" {
				current += " "
			}
			current += text
		}
	}
	flush()
	return result
}

func currentHead(dir string) string {
	var r Result
	_ = readJSON(filepath.Join(dir, "result.json"), &r)
	return r.Head
}
func resetValidation(dir string) error {
	records := validationRecords(dir)
	records = append(records, ValidationRecord{Reset: true, State: "reset"})
	return WriteJSON(filepath.Join(dir, "validation.json"), records)
}

func VisibleByDefault(row Row) bool {
	return row.State == "running" || row.State == "died" || row.State == "needs-review" || row.Validation == "pending" || row.Validation == "running" || row.Validation == "fail"
}
