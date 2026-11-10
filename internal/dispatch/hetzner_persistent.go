package dispatch

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"
)

// ProvisionHetzner serializes first-use adoption and creation across dispatcher
// processes. Run admission itself remains authoritative on the shared receiver.
func ProvisionHetzner(ctx context.Context, h Host) (Machine, error) {
	p, e := NewHetzner(h)
	if e != nil {
		return Machine{}, e
	}
	return p.provision(ctx)
}
func (p *Hetzner) provision(ctx context.Context) (Machine, error) {
	h := p.host
	var unlock func()
	var e error
	for {
		unlock, e = lockBoat(h.Name, "persistent", "provision")
		if e == nil {
			break
		}
		select {
		case <-ctx.Done():
			return Machine{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	defer unlock()
	m, e := p.Create(ctx, MachineSpec{Name: "kit-" + h.Name, Host: h.Name, From: h.From, Size: h.Size, Region: h.Region, Labels: map[string]string{"hev.dispatch": h.Name, "hev.mode": "persistent"}})
	if e != nil {
		return m, e
	}
	if e = p.WaitReady(ctx, m.ID); e != nil {
		return m, e
	}
	config := fmt.Sprintf("cap = %d\nworkspace = '/home/user/dispatch-source'\n[hosts.%s]\nlocal = true\nnamespace = %q\n", h.Cap, h.Name, h.Namespace)
	_, e = p.Exec(ctx, m.ID, "mkdir -p /home/user/.config/hev /home/user/dispatch-source && printf %s "+Quote(base64.StdEncoding.EncodeToString([]byte(config)))+" | base64 -d > /home/user/.config/hev/dispatch.toml")
	return m, e
}
func hetznerPersistentRPC(ctx context.Context, h Host, q Request) (Response, error) {
	p, e := NewHetzner(h)
	if e != nil {
		return Response{}, e
	}
	return hetznerPersistentWithProvider(ctx, h, q, p)
}
func hetznerPersistentWithProvider(ctx context.Context, h Host, q Request, p *Hetzner) (Response, error) {
	if q.AdmissionOnly {
		return Response{}, nil
	}
	ms, e := p.List(ctx, h.Name)
	if e != nil {
		return Response{}, e
	}
	if len(ms) > 1 {
		return Response{}, fmt.Errorf("ambiguous persistent Hetzner host")
	}
	var m Machine
	if len(ms) == 0 {
		if q.Action != "start" {
			return Response{Cap: h.Cap, Free: h.Cap}, nil
		}
		m, e = p.provision(ctx)
		if e != nil {
			return Response{}, e
		}
	} else {
		m = ms[0]
	}
	if q.Action == "start" {
		m, e = p.provision(ctx)
		if e != nil {
			return Response{}, e
		}
		if q.AdmissionOnly {
			return Response{}, nil
		}
		if q.Order != "" {
			o, e := Parse(q.Order)
			if e != nil {
				return Response{}, e
			}
			q.Meta.Order = o
		}
		if !ValidName(q.Name) || q.Meta.Name != q.Name {
			return Response{}, fmt.Errorf("invalid Hetzner order name")
		}
		repo, e := boatRepo(q.Meta.Repo)
		if e != nil {
			return Response{}, e
		}
		// Each order gets a separate source clone; receiver applies its normal cap.
		source := "/home/user/dispatch-source/" + q.Name
		_, e = p.Exec(ctx, m.ID, "git clone -- "+Quote(repo)+" "+Quote(source))
		if e != nil {
			return Response{}, e
		}
		q.Meta.Ephemeral = true
		q.Meta.Repo = source
		q.Meta.ConfigPath = "/home/user/.config/hev/dispatch.toml"
		q.Cap = h.Cap
		if e = activateHetznerRuntime(ctx, p, m.ID, h); e != nil {
			return Response{}, e
		}
	}
	return providerRPC(ctx, p, m.ID, q)
}

// Runtime credentials travel only over pinned SSH into /run, never cloud-init
// or the provider API. The provider token is deliberately not forwarded.
func activateHetznerRuntime(ctx context.Context, p *Hetzner, id string, h Host) error {
	var env strings.Builder
	for _, key := range []string{"HEVLAYER_API_KEY", "HEVLAYER_NAMESPACE", "HEVLAYER_ENDPOINT", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GH_TOKEN", "GITHUB_TOKEN"} {
		if key == h.APIKeyEnv {
			continue
		}
		if value := os.Getenv(key); value != "" {
			env.WriteString("export " + key + "=" + Quote(value) + "\n")
		}
	}
	_, e := p.Exec(ctx, id, "umask 077; f=$(mktemp); trap 'rm -f \"$f\"' EXIT; printf %s "+Quote(base64.StdEncoding.EncodeToString([]byte(env.String())))+" | base64 -d > \"$f\" && sudo install -o user -m 600 \"$f\" /run/hev-runtime.env")
	if e != nil {
		return e
	}
	if os.Getenv("HEVLAYER_API_KEY") != "" {
		return activateBoatCapture(ctx, p, id)
	}
	return nil
}
