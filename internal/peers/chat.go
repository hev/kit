package peers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/pflag"
)

// A chat token carries routing, so a headless recipient can reply even on another
// host. Messages use the ordinary durable inbox transport and its provenance.
type chatRoute struct {
	Members []string `json:"members"`
	Title   string   `json:"title"`
	Nonce   string   `json:"nonce"`
}

func decodeChat(id string) (chatRoute, error) {
	var r chatRoute
	if len(id) > 16384 {
		return r, fmt.Errorf("chat token too long")
	}
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if len(r.Members) < 2 || len(r.Members) > 32 {
		return r, fmt.Errorf("invalid chat members")
	}
	for _, m := range r.Members {
		if !strings.Contains(m, "/") || strings.ContainsAny(m, "\r\n\x00 \t") {
			return r, fmt.Errorf("invalid member")
		}
	}
	return r, nil
}
func chatIDs(text string) []string {
	var ids []string
	for _, line := range strings.Split(text, "\n") {
		at := strings.Index(line, "[hev chat ")
		if at < 0 {
			continue
		}
		id, _, ok := strings.Cut(line[at+10:], "]")
		if ok {
			if _, err := decodeChat(id); err == nil {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
func (e *Engine) chat(ctx context.Context, args []string, in io.Reader, out, errout io.Writer) error {
	fs := pflag.NewFlagSet("chat", pflag.ContinueOnError)
	fs.SetOutput(errout)
	title := fs.String("title", "", "Chat title")
	message := fs.StringP("message", "m", "", "Opening message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	args = fs.Args()
	action := "ls"
	if len(args) > 0 {
		action = args[0]
		args = args[1:]
	}
	ps, err := e.list(ctx, true)
	if err != nil {
		return err
	}
	own, err := e.owner(ctx, ps)
	if err != nil {
		return err
	}
	history := e.path(own.Session + ".chats")
	discover := func() ([]Message, error) {
		existing, err := e.store().Rows(history)
		if err != nil {
			return nil, err
		}
		known := map[string]bool{}
		for _, m := range existing {
			if id, ok := m["chat"].(string); ok {
				known[id] = true
			}
		}
		text := ""
		if own.Run {
			b, err := os.ReadFile(filepath.Join(dispatch.Root(), own.Session, "inbox.md"))
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			text = string(b)
		} else {
			b, err := os.ReadFile(e.path(own.Session))
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			text = string(b)
		}
		// JSON inbox envelopes escape newlines but the token itself is unchanged.
		for _, id := range chatIDs(strings.ReplaceAll(text, `\n`, "\n")) {
			if known[id] {
				continue
			}
			known[id] = true
			if err := os.MkdirAll(filepath.Dir(history), 0700); err != nil {
				return nil, err
			}
			if err := e.store().Append(history, Message{"chat": id}); err != nil {
				return nil, err
			}
		}
		return e.store().Rows(history)
	}
	switch action {
	case "ls":
		rs, err := discover()
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, m := range rs {
			id, _ := m["chat"].(string)
			if seen[id] {
				continue
			}
			seen[id] = true
			r, err := decodeChat(id)
			if err == nil {
				fmt.Fprintf(out, "%s %s %s\n", id, r.Title, strings.Join(r.Members, ", "))
			}
		}
		return nil
	case "show":
		if len(args) != 1 {
			return fmt.Errorf("chat show ID")
		}
		if _, err := decodeChat(args[0]); err != nil {
			return err
		}
		rs, err := discover()
		if err != nil {
			return err
		}
		for _, m := range rs {
			if m["chat"] == args[0] && m["message"] != nil {
				fmt.Fprintln(out, m["message"])
			}
		}
		if own.Run {
			b, _ := os.ReadFile(filepath.Join(dispatch.Root(), own.Session, "inbox.md"))
			for _, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, args[0]) {
					fmt.Fprintln(out, line)
				}
			}
		} else {
			rs, err = e.store().Rows(e.path(own.Session))
			if err != nil {
				return err
			}
			for _, m := range rs {
				b, _ := json.Marshal(m)
				if strings.Contains(string(b), args[0]) {
					fmt.Fprintln(out, string(b))
				}
			}
		}
		return nil
	case "new", "say":
		var id, body string
		if action == "new" {
			if len(args) < 1 || *message == "" {
				return fmt.Errorf("chat new MEMBER... --title TITLE -m MESSAGE")
			}
			members := []string{own.Address}
			seen := map[string]bool{own.Address: true}
			for _, m := range args {
				if !strings.Contains(m, "/") {
					m = e.Host + "/" + m
				}
				if !seen[m] {
					members = append(members, m)
					seen[m] = true
				}
			}
			nonce := make([]byte, 8)
			if _, err := rand.Read(nonce); err != nil {
				return err
			}
			b, _ := json.Marshal(chatRoute{members, *title, hex.EncodeToString(nonce)})
			id = base64.RawURLEncoding.EncodeToString(b)
			body = *message
		} else {
			if len(args) < 2 {
				return fmt.Errorf("chat say ID MESSAGE")
			}
			id = args[0]
			body = strings.Join(args[1:], " ")
		}
		if body == "-" {
			b, err := io.ReadAll(in)
			if err != nil {
				return err
			}
			body = string(b)
		}
		r, err := decodeChat(id)
		if err != nil {
			return err
		}
		member := false
		for _, m := range r.Members {
			member = member || m == own.Address
		}
		if !member {
			return fmt.Errorf("current peer is not a chat member")
		}
		body = "[hev chat " + id + "] " + strings.ReplaceAll(body, "\n", "\n[hev chat "+id+"] ")
		if err := os.MkdirAll(filepath.Dir(history), 0700); err != nil {
			return err
		}
		if err := e.store().Append(history, Message{"chat": id, "message": own.Address + ": " + body}); err != nil {
			return err
		}
		var failures []string
		for _, m := range r.Members {
			if m == own.Address {
				continue
			}
			if err = e.Run(ctx, []string{"send", m, body}, in, out, errout); err != nil {
				failures = append(failures, m+": "+err.Error())
			}
		}
		fmt.Fprintln(out, id)
		if len(failures) > 0 {
			return fmt.Errorf("chat delivery failures: %s; inspect before retrying", strings.Join(failures, "; "))
		}
		return nil
	default:
		return fmt.Errorf("use chat new, ls, show or say")
	}
}
