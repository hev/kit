package secrets

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ReadError is a provider's classified, value-free read failure. It carries
// only fixed reason text, the failing reference's caller-chosen NAME (filled
// in by the cache), a fixed hint and an exit code: never a value, a token, a
// reference or raw upstream output.
type ReadError struct {
	// Provider is the store's human name ("1Password").
	Provider string
	// Reason is a short fixed phrase, e.g. "item deleted or archived".
	Reason string
	// Hint is a fixed suggestion, or "".
	Hint string
	// Index is the failing reference's position in the uris given to Read,
	// or -1 when the answer does not say which one failed.
	Index int
	// Count is the number of references in the failed batch.
	Count int
	// Reached reports whether the provider answered at all: "yes" (it
	// refused this request), "no" (network or timeout) or "unknown".
	Reached string
	// Name is the failing reference's NAME, set by the cache from Index.
	Name string
}

func (e *ReadError) Error() string {
	var b strings.Builder
	b.WriteString(e.Provider + " secret resolution failed: " + e.Reason)
	switch {
	case e.Name != "":
		b.WriteString(" (reference " + e.Name + ")")
	case e.Index < 0 && e.Count > 1 && e.refScoped():
		fmt.Fprintf(&b, " (one of %d references)", e.Count)
	}
	if e.Hint != "" {
		b.WriteString("; " + e.Hint)
	}
	return b.String()
}

func (e *ReadError) refScoped() bool {
	return strings.HasPrefix(e.Reason, "item") || strings.HasPrefix(e.Reason, "vault") || strings.HasPrefix(e.Reason, "field")
}

// opItemID is the shape of a 1Password item id; anything else in the item
// segment of a reference is a title.
var opItemID = regexp.MustCompile(`^[a-z0-9]{26}$`)

const archivedHint = "duplicate archived items with this title? reference the item by id"

type opClass struct {
	reason  string
	reached string
	segment int // 0 vault, 1 item, 2 field (last), -1 none
	any     []string
}

// opClasses are checked in order against the sanitized, lower-cased stderr.
// Field and vault come before item: their messages also name the item.
var opClasses = []opClass{
	{"field not found", "yes", 2, []string{"isn't a field", "is not a field", "could not find field", "field not found", "no field", "does not have a field", "field does not exist"}},
	{"vault not found or no access", "yes", 0, []string{"isn't a vault", "is not a vault", "could not find vault", "vault not found", "no vault", "don't have access", "do not have access", "permission denied", "forbidden", "not authorized to access"}},
	{"item deleted or archived", "yes", 1, []string{"deleted or archived", "has been archived", "has been deleted", "in the trash"}},
	{"item title matches more than one item", "yes", 1, []string{"more than one item matches", "multiple items"}},
	{"item not found", "yes", 1, []string{"isn't an item", "is not an item", "could not find item", "item not found", "no item found", "no item matches"}},
	{"authentication failed (service-account token invalid, expired or revoked)", "yes", -1, []string{"unauthorized", "authentication", "invalid token", "token is invalid", "token has expired", "token expired", "expired token", "invalid service account", "signin", "sign in", "signed in", "session expired", "invalid credentials"}},
	{"network error or timeout reaching 1Password", "no", -1, []string{"dial tcp", "no such host", "connection refused", "connection reset", "i/o timeout", "timeout", "timed out", "deadline exceeded", "network is unreachable", "tls handshake", "temporary failure in name resolution"}},
}

// classifyOpInject turns a failed `op inject` into a *ReadError. raw is the
// unsanitized stderr; it is used only to pick which reference failed and
// never copied into the result.
func classifyOpInject(ctx context.Context, runErr error, raw string, uris []string) *ReadError {
	e := &ReadError{Provider: "1Password", Index: -1, Count: len(uris), Reached: "unknown"}
	lower := strings.ToLower(opRefText.ReplaceAllString(raw, ""))
	var ee *exec.ExitError
	var class *opClass
	switch {
	case ctx.Err() != nil:
		class = &opClasses[len(opClasses)-1]
	case errors.As(runErr, &ee):
		for i := range opClasses {
			for _, s := range opClasses[i].any {
				if strings.Contains(lower, s) {
					class = &opClasses[i]
					break
				}
			}
			if class != nil {
				break
			}
		}
	default:
		e.Reason = "op CLI could not be started (is it installed and on PATH?)"
		e.Reached = "no"
		return e
	}
	if class == nil {
		e.Reason = fmt.Sprintf("op inject exited %d (unrecognized error; output withheld)", ee.ExitCode())
		return e
	}
	e.Reason, e.Reached = class.reason, class.reached
	if class.segment >= 0 {
		e.Index = failingRef(raw, lower, uris, class.segment)
	}
	if class.segment == 1 && class.reason != "item not found" {
		title := false
		for i, u := range uris {
			if (e.Index < 0 || e.Index == i) && !opItemID.MatchString(opSegment(u, 1)) {
				title = true
			}
		}
		if title {
			e.Hint = archivedHint
			if class.reason != "item deleted or archived" {
				e.Hint = "reference the item by id"
			}
		}
	}
	return e
}

// opSegment returns the vault (0), item (1) or field (2, the last segment) of
// an op:// reference, lower-cased.
func opSegment(uri string, seg int) string {
	parts := strings.Split(strings.TrimPrefix(uri, "op://"), "/")
	if seg >= 2 {
		seg = len(parts) - 1
	}
	if seg >= len(parts) {
		return ""
	}
	return strings.ToLower(parts[seg])
}

// failingRef picks the reference op complained about: the one whose full
// reference appears in the raw answer, else the one whose vault, item or
// field label appears as a whole word in the sanitized answer, else the only
// reference. It returns -1 when the answer is ambiguous.
func failingRef(raw, lower string, uris []string, seg int) int {
	if len(uris) == 1 {
		return 0
	}
	pick := func(match func(string) bool) int {
		found := -1
		for i, u := range uris {
			if match(u) {
				if found >= 0 && uris[found] != u {
					return -1
				}
				if found < 0 {
					found = i
				}
			}
		}
		return found
	}
	seen := map[string]bool{}
	for _, m := range opRefText.FindAllString(raw, -1) {
		seen[strings.TrimRight(m, ".,;:)]")] = true
	}
	if i := pick(func(u string) bool { return seen[u] }); i >= 0 {
		return i
	}
	return pick(func(u string) bool {
		label := opSegment(u, seg)
		if label == "" {
			return false
		}
		return regexp.MustCompile(`(^|[^a-z0-9_.-])` + regexp.QuoteMeta(label) + `($|[^a-z0-9_-])`).MatchString(lower)
	})
}
