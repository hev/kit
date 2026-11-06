package dispatch

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

func LocalSender() string {
	u, e := user.Current()
	if e == nil && u.Username != "" {
		return "local:" + u.Username
	}
	return "local:unknown"
}

// HandleAs is only called with an identity authenticated from the host-owned
// SSH access record. No field of Request can select the authenticated sender.
func HandleAs(q Request, sender string) (Response, error) {
	if sender == "" {
		return Response{}, fmt.Errorf("authenticated sender required")
	}
	if err := authorizeSender(&q, sender); err != nil {
		return Response{}, err
	}
	r, e := Handle(q)
	if q.Action == "list" {
		rows := r.Rows[:0]
		names := []string{}
		for _, row := range r.Rows {
			if row.Sender == sender && row.Kind == "run" && row.Forward == nil && row.Parent == "" {
				rows = append(rows, row)
				names = append(names, row.Name)
			}
		}
		r.Rows, r.Names = rows, names
	}
	return r, e
}
func authorizeSender(q *Request, sender string) error {
	if q.Orchestrator != "" || q.Meta.Parent != "" || q.Meta.Forward != nil || q.Meta.Imported || q.Meta.ParentHostName != "" || q.Meta.ParentHost.SSH != "" {
		return fmt.Errorf("teammate dispatch cannot delegate or forward")
	}
	q.FromValidation = false
	q.Cap = 0
	q.AdmissionOnly = false
	q.ReserveClass, q.ClassToken = false, ""
	q.Transfer = nil
	q.ValidationRecord = ValidationRecord{}
	switch q.Action {
	case "start":
		if q.Order != "" {
			order, err := Parse(q.Order)
			if err != nil {
				return err
			}
			if order.Validation == "browser" {
				return fmt.Errorf("teammate dispatch cannot request validation; ask the owner")
			}
		}
		// Discard all client launch authority; target admission and launch resolution
		// still decide repository, harness, environment and workspace.
		cfg, e := targetPolicy()
		if e != nil {
			return e
		}
		hostname, _ := os.Hostname()
		host := legacyTargetHost(cfg.Hosts, hostname)
		if host == "" {
			host = hostname
		}
		q.Meta = Meta{Order: Order{Name: q.Name, Host: host}, Sender: sender, RequestedBy: sender}
		q.Workspace = ""
		return nil
	case "list":
		return nil
	case "say", "peer-send", "logs", "show", "stop":
		if !ValidName(q.Name) {
			return fmt.Errorf("invalid name")
		}
		var m Meta
		if readJSON(filepath.Join(Root(), q.Name, "meta.json"), &m) != nil || m.Sender != sender || m.Forward != nil || m.Parent != "" {
			return fmt.Errorf("permission denied: run is not owned by %s", sender)
		}
		if q.Action == "say" {
			q.Action = "peer-send"
		}
		return nil
	default:
		return fmt.Errorf("permission denied: action %s is unavailable to teammates", q.Action)
	}
}

// SenderAttribution uses the same worktree boundary as order attribution.
func SenderAttribution(cwd string) string {
	name := attributionName(cwd)
	if name == "" {
		return ""
	}
	var m Meta
	if readJSON(filepath.Join(Root(), name, "meta.json"), &m) == nil {
		return m.Sender
	}
	return ""
}

// requester keeps older metadata useful while new starts always persist both fields.
func requester(m Meta) string {
	if m.RequestedBy != "" {
		return m.RequestedBy
	}
	return m.Sender
}
func requestedPrompt(prompt string, m Meta) string {
	if name := requester(m); name != "" {
		return prompt + " Requested by " + name + " (recorded in ../meta.json as requested_by). When opening a PR, include Requested by " + name + " in its description."
	}
	return prompt
}
