// Package peersview provides a read-only hosted presence and message view.
package peersview

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/hev/kit/pkg/kitapi"
)

// Reader must be scoped to the authenticated viewer's team by the host. It
// deliberately has no send/append methods and never accesses local peer state.
type Reader interface {
	PeerPresence(context.Context) (kitapi.PresencePage, error)
	PeerMessages(context.Context, url.Values) (kitapi.PeerMessagesPage, error)
}
type Principal struct{ ID string }
type HostedOptions struct {
	BasePath  string
	Principal func(*http.Request) (Principal, error)
}

var page = template.Must(template.New("peers").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Peers</title></head><body><h1>Peers</h1><main><section aria-label="Live machines and sessions"><h2>Live machines and sessions</h2>{{range .Presence.Machines}}<article><h3>{{.Host}}</h3><p>{{.OS}} · {{.Version}} · expires {{.ExpiresAt}}</p><ul>{{range .Sessions}}<li>{{.Name}} · {{.Harness}} {{.Model}} · {{.Project}} {{.Branch}} · {{.State}}{{if .StartedAt}} · started {{.StartedAt}}{{end}}{{if .LastActiveAt}} · active {{.LastActiveAt}}{{end}}</li>{{end}}</ul></article>{{else}}<p>No live machines</p>{{end}}</section><section aria-label="Message log"><h2>Message log</h2><form method="get"><label>Machine <input name="machine" value="{{.Machine}}"></label><label>Session (HOST/NAME) <input name="session" value="{{.Session}}"></label><label>Chat <input name="chat" value="{{.Chat}}"></label><button>Filter</button></form>{{range .Log.Messages}}<article><p>{{.Time}} · {{.Sender}} → {{range .Recipients}}{{.}} {{end}} · {{.Kind}} {{.ChatID}} {{.Title}}</p><pre>{{.Body}}</pre></article>{{else}}<p>No messages</p>{{end}}{{if .Next}}<a href="{{.Next}}">Older messages</a>{{end}}</section></main></body></html>`))

// NewHosted consumes its own BasePath (mount without StripPrefix). GET/HEAD
// only; no write route or send form. Reader errors are never reflected.
func NewHosted(reader Reader, opts HostedOptions) (http.Handler, error) {
	if reader == nil || opts.Principal == nil {
		return nil, errors.New("peers: reader and principal required")
	}
	base := strings.TrimRight(opts.BasePath, "/")
	if base != "" && (!strings.HasPrefix(base, "/") || strings.ContainsAny(base, "?#\\\"'<>% \t\r\n") || strings.Contains(base, "..") || strings.Contains(base, "//")) {
		return nil, errors.New("peers: invalid base path")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		principal, e := opts.Principal(r)
		if e != nil || strings.TrimSpace(principal.ID) == "" {
			http.Error(w, "authentication required", 401)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", 405)
			return
		}
		if r.URL.Path != base && r.URL.Path != base+"/" {
			http.NotFound(w, r)
			return
		}
		presence, e := reader.PeerPresence(r.Context())
		if e != nil {
			http.Error(w, "presence unavailable", 502)
			return
		}
		q := url.Values{}
		for _, k := range []string{"machine", "session", "chat", "cursor", "limit"} {
			if v := r.URL.Query().Get(k); v != "" {
				q.Set(k, v)
			}
		}
		log, e := reader.PeerMessages(r.Context(), q)
		if e != nil {
			http.Error(w, "message log unavailable", 502)
			return
		}
		// Copy before sanitizing: readers may share cached objects across requests.
		presence.Machines = append([]kitapi.PeerMachine(nil), presence.Machines...)
		for i := range presence.Machines {
			m := &presence.Machines[i]
			m.AuthorID = ""
			m.Host = safe(m.Host)
			m.Version = safe(m.Version)
			m.OS = safe(m.OS)
			m.Sessions = append([]kitapi.PeerSession(nil), m.Sessions...)
			for j := range m.Sessions {
				s := &m.Sessions[j]
				s.Project = safe(s.Project)
				s.Name = safe(s.Name)
				s.Harness = safe(s.Harness)
				s.Model = safe(s.Model)
				s.Branch = safe(s.Branch)
				s.State = safe(s.State)
			}
		}
		log.Messages = append([]kitapi.PeerMessage(nil), log.Messages...)
		for i := range log.Messages {
			m := &log.Messages[i]
			m.Body = safe(m.Body)
			m.Sender = safe(m.Sender)
			m.ChatID = safe(m.ChatID)
			m.Title = safe(m.Title)
			m.Kind = safe(m.Kind)
			m.Recipients = append([]string(nil), m.Recipients...)
			for j := range m.Recipients {
				m.Recipients[j] = safe(m.Recipients[j])
			}
		}
		next := ""
		if log.NextCursor != "" {
			q.Set("cursor", log.NextCursor)
			clean := true
			for _, values := range q {
				for _, value := range values {
					if safe(value) != value {
						clean = false
					}
				}
			}
			if clean {
				next = base + "/?" + q.Encode()
			}
		}
		data := struct {
			Presence                     kitapi.PresencePage
			Log                          kitapi.PeerMessagesPage
			Machine, Session, Chat, Next string
		}{presence, log, safe(q.Get("machine")), safe(q.Get("session")), safe(q.Get("chat")), next}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != "HEAD" {
			_ = page.Execute(w, data)
		}
	}), nil
}

// Stored text is escaped by html/template; additionally suppress accidental
// local paths, loopback links and kit keys, even in a message body.
var privateText = regexp.MustCompile(`(?i)(?:https?://(?:localhost|127\.[0-9.]+|\[::1\])[^\s<>]*|hk_[A-Za-z0-9_-]+)`)

// Scan punctuation-delimited candidates, preserving ordinary URI schemes and
// peer/repository slash names. Decode at most twice; never render decoded text.
func localCandidate(s string) bool {
	for round := 0; round < 3; round++ {
		if localDecoded(s) {
			return true
		}
		if round == 2 {
			break
		}
		decoded, err := url.PathUnescape(s)
		if err != nil || decoded == s {
			break
		}
		s = decoded
	}
	return false
}
func localDecoded(s string) bool {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	// Re-tokenize decoded whitespace and plus as well as punctuation. Inspect
	// label suffixes once; ordinary URI schemes own the rest of their token.
	for i := 0; i < len(s); {
		if displaySeparator(s, i) {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && !displaySeparator(s, j) {
			j++
		}
		original := s[i:j]
		lower := []byte(original)
		for n, b := range lower {
			if b >= 'A' && b <= 'Z' {
				lower[n] = b + ('a' - 'A')
			}
		}
		token := string(lower)
		for k := 0; k < len(token); k++ {
			if k != 0 && token[k-1] != ':' && token[k-1] != '=' {
				continue
			}
			// Preserve embedded mixed-case drive-like prose (abcC:/...),
			// which has no drive boundary before the capital letter.
			if k >= 3 && original[k-1] == ':' && original[k-2] >= 'A' && original[k-2] <= 'Z' && original[k-3] >= 'a' && original[k-3] <= 'z' {
				continue
			}
			v := token[k:]
			if strings.HasPrefix(v, "file:") && len(v) > 5 {
				return true
			}
			if len(v) > 2 && v[0] >= 'a' && v[0] <= 'z' && v[1] == ':' && (v[2] == '/' || v[2] == '\\') {
				return true
			}
			for _, prefix := range []string{"/", `\\`, "./", "../", "$home/", "$home\\", "${home}/", "${home}\\", "%userprofile%/", "%userprofile%\\", "%homepath%/", "%homepath%\\"} {
				if strings.HasPrefix(v, prefix) {
					return true
				}
			}
			if strings.HasPrefix(v, "~") && strings.ContainsAny(v, `/\`) {
				return true
			}
			// A scheme starts with a letter and continues with URI scheme
			// characters. Preserve its entire URL, including query separators.
			if len(v) > 0 && v[0] >= 'a' && v[0] <= 'z' {
				n := 1
				for n < len(v) && ((v[n] >= 'a' && v[n] <= 'z') || (v[n] >= '0' && v[n] <= '9') || strings.ContainsRune("+.-", rune(v[n]))) {
					n++
				}
				if strings.HasPrefix(v[n:], "://") {
					break
				}
			}
		}
		i = j
	}
	return false
}

func displaySeparator(s string, i int) bool {
	return strings.ContainsRune(" +\t\r\n\v\f<>\"'`[](),;", rune(s[i])) ||
		(s[i] == '{' && (i == 0 || s[i-1] != '$')) ||
		(s[i] == '}' && !(i >= 6 && strings.EqualFold(s[i-6:i], "${home")))
}

const maxDisplayBytes = 16 * 1024

func safe(s string) string {
	// Bound all inspection and decoding to 16 KiB per field. No prefix
	// rescans: O(min(len(s), maxDisplayBytes)) work and bounded allocation.
	truncated := len(s) > maxDisplayBytes
	if truncated {
		s = s[:maxDisplayBytes]
	}
	s = privateText.ReplaceAllString(s, "[private]")
	var out strings.Builder
	for i := 0; i < len(s); {
		if displaySeparator(s, i) {
			out.WriteByte(s[i])
			i++
			continue
		}
		j := i + 1
		for j < len(s) && !displaySeparator(s, j) {
			j++
		}
		token := s[i:j]
		if localCandidate(token) && !(i > 0 && s[i-1] == '<' && (strings.EqualFold(token, "/script") || strings.EqualFold(token, "/pre"))) {
			out.WriteString("[private]")
		} else {
			out.WriteString(token)
		}
		i = j
	}
	if truncated {
		out.WriteString("[truncated]")
	}
	return out.String()
}
