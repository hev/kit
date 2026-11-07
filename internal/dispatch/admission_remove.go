package dispatch

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// RemoveRepo drops an origin from [admission] repos. The clone stays in the
// workspace and running runs continue; only new dispatches are refused.
// Removing an origin that is not admitted succeeds without a change.
func RemoveRepo(raw string, caller RepoCaller) (Response, error) {
	r := Response{}
	if caller.Sender == "" {
		return r, fmt.Errorf("authenticated sender required")
	}
	if caller.Restricted && !caller.CanAdd {
		return r, fmt.Errorf("permission denied: your access does not include removing repos (the owner can run: hev nodes allow NAME %s)", RepoCapability)
	}
	raw = strings.TrimSpace(raw)
	a, err := parseAddURL(raw, true)
	if err != nil {
		// Owners may remove any entry, including ones a teammate could not add.
		if caller.Restricted || raw == "" {
			return r, err
		}
		a = addURL{Clone: raw, Identity: repoIdentity(raw)}
	}
	policy, err := targetPolicy()
	if err != nil {
		return r, fmt.Errorf("target admission config: %w", err)
	}
	if caller.Restricted && !addFromAllows(policy.Admission.AddFrom, a) {
		return r, fmt.Errorf("permission denied: %s was not added from an allowed organization; ask the owner", a.Clone)
	}
	removed, err := unadmitRepo(ConfigPath(""), a, caller.Sender)
	if err != nil {
		return r, err
	}
	if len(removed) == 0 {
		r.Message = fmt.Sprintf("%s is not admitted; nothing to remove", a.Clone)
		return r, nil
	}
	r.Message = fmt.Sprintf("removed %s from admission; new dispatches to it are refused", strings.Join(removed, ", "))
	if a.Name != "" {
		r.Message += fmt.Sprintf("; the clone %s stays in the workspace and runs already started keep running", a.Name)
	} else {
		r.Message += "; any clone stays in the workspace and runs already started keep running"
	}
	return r, nil
}

// unadmitRepo removes every [admission] repos entry with a's identity under
// the dispatch lock and returns the entries it removed.
func unadmitRepo(cfgPath string, a addURL, sender string) ([]string, error) {
	if err := os.MkdirAll(Root(), 0700); err != nil {
		return nil, err
	}
	unlock, err := lockRoot()
	if err != nil {
		return nil, err
	}
	defer unlock()
	text, err := os.ReadFile(cfgPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("this host admits every workspace repository (no [admission] repos list), so %s cannot be removed; ask the owner", a.Clone)
	}
	if err != nil {
		return nil, err
	}
	var current struct {
		Admission Admission `toml:"admission"`
	}
	if _, err = toml.Decode(string(text), &current); err != nil {
		return nil, fmt.Errorf("target admission config: %w", err)
	}
	if current.Admission.Repos == nil {
		return nil, fmt.Errorf("this host admits every workspace repository (no [admission] repos list), so %s cannot be removed; ask the owner", a.Clone)
	}
	removed := []string{}
	for _, existing := range current.Admission.Repos {
		if repoIdentity(existing) == a.Identity {
			removed = append(removed, existing)
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}
	edited, err := removeAdmissionRepos(string(text), a.Identity)
	if err != nil {
		return nil, err
	}
	if err = writeFileAtomic(cfgPath, []byte(edited)); err != nil {
		return nil, err
	}
	for _, origin := range removed {
		if err = appendRepoLedger(cfgPath, repoLedgerEntry{Action: "remove", Origin: origin, Name: a.Name, RemovedBy: sender, RemovedAt: time.Now().UTC()}); err != nil {
			return removed, fmt.Errorf("removed %s but could not record attribution: %w", origin, err)
		}
	}
	return removed, nil
}

// removeAdmissionRepos deletes the [admission] repos elements whose identity
// matches, leaving every other byte in place where it can. An element alone on
// its line takes the whole line, including its comment. The result is proven
// by decoding to differ only by those elements.
func removeAdmissionRepos(text, identity string) (string, error) {
	open, close, err := findAdmissionRepos(text)
	if err != nil {
		return "", err
	}
	refuse := fmt.Errorf("cannot safely edit [admission] repos in the target config; remove %q by hand", identity)
	type span struct{ start, end int }
	cuts := []span{}
	for j := open + 1; j < close; j++ {
		switch text[j] {
		case '#':
			j = lineEndAt(text, j) - 1
		case '"', '\'':
			end, err := skipString(text, j)
			if err != nil {
				return "", err
			}
			var v struct {
				V string `toml:"v"`
			}
			if _, err = toml.Decode("v = "+text[j:end], &v); err != nil {
				return "", refuse
			}
			if repoIdentity(v.V) == identity {
				start, stop := elementSpan(text, open, j, end)
				cuts = append(cuts, span{start, stop})
			}
			j = end - 1
		}
	}
	sort.Slice(cuts, func(i, k int) bool { return cuts[i].start < cuts[k].start })
	var b strings.Builder
	at := 0
	for _, c := range cuts {
		if c.start > at {
			b.WriteString(text[at:c.start])
		}
		at = max(at, c.end)
	}
	b.WriteString(text[at:])
	edited := b.String()
	if !sameExceptRemoved(text, edited, identity) {
		return "", refuse
	}
	return edited, nil
}

// elementSpan is the text to delete for the element at [start, end).
func elementSpan(text string, open, start, end int) (int, int) {
	after := end
	for after < len(text) && (text[after] == ' ' || text[after] == '\t') {
		after++
	}
	comma := after < len(text) && text[after] == ','
	if comma {
		after++
		for after < len(text) && (text[after] == ' ' || text[after] == '\t') {
			after++
		}
	}
	lineStart := strings.LastIndexByte(text[:start], '\n') + 1
	eol := lineEndAt(text, after)
	rest := strings.TrimRight(text[after:eol], "\r")
	if lineStart > open && strings.TrimLeft(text[lineStart:start], " \t") == "" && (rest == "" || rest[0] == '#') {
		return lineStart, min(eol+1, len(text))
	}
	if comma {
		return start, after
	}
	// The last element on a shared line also takes the comma before it.
	before := start
	for before > open+1 && (text[before-1] == ' ' || text[before-1] == '\t') {
		before--
	}
	if text[before-1] == ',' {
		return before - 1, end
	}
	return start, end
}

func sameExceptRemoved(before, after, identity string) bool {
	var a, b map[string]any
	if _, err := toml.Decode(before, &a); err != nil {
		return false
	}
	if _, err := toml.Decode(after, &b); err != nil {
		return false
	}
	beforeAdmission, ok := a["admission"].(map[string]any)
	if !ok {
		return false
	}
	afterAdmission, ok := b["admission"].(map[string]any)
	if !ok {
		return false
	}
	beforeRepos, ok := beforeAdmission["repos"].([]any)
	if !ok {
		return false
	}
	afterRepos, ok := afterAdmission["repos"].([]any)
	if !ok {
		return false
	}
	kept := []any{}
	for _, repo := range beforeRepos {
		if s, isString := repo.(string); !isString || repoIdentity(s) != identity {
			kept = append(kept, repo)
		}
	}
	if len(kept) != len(afterRepos) || (len(kept) > 0 && !reflect.DeepEqual(kept, afterRepos)) {
		return false
	}
	beforeAdmission["repos"], afterAdmission["repos"] = nil, nil
	return reflect.DeepEqual(a, b)
}
