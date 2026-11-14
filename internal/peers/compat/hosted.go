package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hev/kit/internal/hostedpeers"
	"github.com/hev/kit/internal/version"
	"github.com/hev/kit/pkg/kitapi"
)

func (p *P) hostedBeacon(rows []*Row) {
	c, e := hostedpeers.Load()
	if e != nil || !c.SignedIn() {
		return
	}
	if c.Presence {
		sessions := []kitapi.PeerSession{}
		for _, r := range rows {
			if r.Agent == "" {
				continue
			}
			s := kitapi.PeerSession{Name: r.Session, Harness: r.Agent, Project: filepath.Base(r.Cwd), State: p.state(r)}
			if at, ok := num(r.Activity); ok && at > 0 {
				t := time.Unix(int64(at), 0).UTC()
				s.LastActiveAt = &t
			}
			if at := p.started(r.AgentPID); at > 0 {
				t := time.Unix(int64(at), 0).UTC()
				s.StartedAt = &t
			}
			s.Branch = p.branch(r.Cwd)
			sessions = append(sessions, s)
		}
		if e = c.QueuePresence(kitapi.PresenceIn{Host: p.selfLabel(), Version: version.Version, OS: runtime.GOOS, ObservedAt: time.Now().UTC(), Sessions: sessions}); e != nil {
			c.WarnOnce(p.err)
		}
	}
	c.Kick()
}
func (p *P) hostedFlush([]string) {
	c, e := hostedpeers.Load()
	if e != nil || !c.SignedIn() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := kitapi.New(c.API, c.Key)
	client.Retries = 0
	if c.Flush(ctx, client) != nil {
		c.WarnOnce(p.err)
	}
}

// hostedRead is additive: the ordinary ls path is untouched.
func (p *P) hostedRead(args []string) {
	c, e := hostedpeers.Load()
	if e != nil || !c.SignedIn() {
		p.die("not signed in; run hev login")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := kitapi.New(c.API, c.Key)
	if contains(args, "--team") {
		out, e := client.PeerPresence(ctx)
		if e != nil {
			p.die(e.Error())
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(p.out, string(b))
		return
	}
	q := mapValues(args)
	out, e := client.PeerMessages(ctx, q)
	if e != nil {
		p.die(e.Error())
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(p.out, string(b))
}

func mapValues(args []string) url.Values {
	q := url.Values{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "log" || a == "--json" {
			continue
		}
		switch a {
		case "--machine", "--session", "--chat", "--cursor", "--limit":
			if i+1 < len(args) {
				i++
				q.Set(a[2:], args[i])
			}
		}
	}
	return q
}
