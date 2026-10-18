package nativehost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const ReceiverRPCVersion = "native-receiver-rpc/v1"
const ReceiverRPCClientVersion = "native-receiver-rpc-client/v1"

// Remote is opt-in transport configuration, never launch or admission authority.
type Remote struct {
	Listen        string  `json:"listen"`
	AllowedNodeID string  `json:"allowed_node_id"`
	AllowedUserID string  `json:"allowed_user_id"`
	Tailscale     FilePin `json:"tailscale"`
	Runtime       Runtime `json:"runtime"`
}
type ReceiverRPCClient struct {
	Version      string  `json:"version"`
	Address      string  `json:"address"`
	ServerNodeID string  `json:"server_node_id"`
	ServerUserID string  `json:"server_user_id"`
	ProfileID    string  `json:"profile_id"`
	ConfigDigest string  `json:"config_digest"`
	Tailscale    FilePin `json:"tailscale"`
	Runtime      Runtime `json:"runtime"`
}
type receiverEnvelope struct {
	Version      string          `json:"version"`
	ProfileID    string          `json:"profile_id"`
	ConfigDigest string          `json:"config_digest"`
	Call         json.RawMessage `json:"call"`
}

func tailAddress(address string) bool {
	a, e := netip.ParseAddrPort(address)
	if e != nil || a.Port() != 19761 {
		return false
	}
	return netip.MustParsePrefix("100.64.0.0/10").Contains(a.Addr()) || netip.MustParsePrefix("fd7a:115c:a1e0::/48").Contains(a.Addr())
}
func rpcRuntime(pin FilePin, r Runtime) error {
	if pinned(pin.Path, pin.Digest, true) != nil || platformRuntime(r) != nil {
		return ErrHeld
	}
	found := false
	for _, f := range r.Files {
		if pinned(f.Path, f.Digest, false) != nil {
			return ErrHeld
		}
		if f == pin {
			found = true
		}
	}
	if !found {
		return ErrHeld
	}
	var path string
	for _, v := range r.Environment {
		if strings.HasPrefix(v, "PATH=") {
			path = strings.TrimPrefix(v, "PATH=")
		}
	}
	return verifyInterpreters(Profile{SupervisorRef: pin.Path, Runtime: r}, path)
}
func (r Remote) verify() error {
	if !tailAddress(r.Listen) || r.AllowedNodeID == "" || r.AllowedUserID == "" {
		return ErrHeld
	}
	return rpcRuntime(r.Tailscale, r.Runtime)
}
func rpcWhoIs(ctx context.Context, pin FilePin, r Runtime, address, node, user string) error {
	if rpcRuntime(pin, r) != nil {
		return ErrHeld
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, pin.Path, "whois", "--json", address)
	c.Dir = r.Directory
	c.Env = append([]string(nil), r.Environment...)
	c.WaitDelay = time.Second
	var out limitedOutput
	c.Stdout = writerOnly{&out}
	c.Stderr = io.Discard
	if c.Run() != nil || out.overflow {
		return ErrHeld
	}
	var result struct {
		Node        struct{ StableID string }
		UserProfile struct{ ID json.Number }
	}
	d := json.NewDecoder(bytes.NewReader(out.Bytes()))
	d.UseNumber()
	if d.Decode(&result) != nil || result.Node.StableID != node || result.UserProfile.ID.String() != user {
		return ErrHeld
	}
	return nil
}
func boundedRead(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, maxFrame+1))
	if e != nil || len(b) == 0 || len(b) > maxFrame {
		return nil, ErrHeld
	}
	return b, nil
}
func receiverCall(b []byte) error {
	var c struct {
		Version      string          `json:"version"`
		Operation    string          `json:"operation"`
		JobID        string          `json:"job_id"`
		Claim        json.RawMessage `json:"claim,omitempty"`
		Binding      json.RawMessage `json:"binding,omitempty"`
		Identity     json.RawMessage `json:"identity,omitempty"`
		EventID      string          `json:"event_id,omitempty"`
		ResultDigest string          `json:"result_digest,omitempty"`
	}
	if strict(b, &c) != nil || c.Version != "native-receiver/v1" || c.JobID == "" {
		return ErrHeld
	}
	switch c.Operation {
	case "start", "inspect", "ack":
		return nil
	}
	return ErrHeld
}
func receiverRPCHandler(profiles []Profile, r Remote) http.Handler {
	slots := make(chan struct{}, 16)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "unknown; inspect original; do not replay", http.StatusServiceUnavailable)
			return
		}
		held := func() { http.Error(w, "unknown; inspect original; do not replay", http.StatusServiceUnavailable) }
		if req.Method != "POST" || req.URL.Path != "/native-receiver/v1" || req.URL.RawQuery != "" || rpcWhoIs(req.Context(), r.Tailscale, r.Runtime, req.RemoteAddr, r.AllowedNodeID, r.AllowedUserID) != nil {
			held()
			return
		}
		b, e := boundedRead(req.Body)
		var envelope receiverEnvelope
		if e != nil || strict(b, &envelope) != nil || envelope.Version != ReceiverRPCVersion || receiverCall(envelope.Call) != nil {
			held()
			return
		}
		var selected *Profile
		for i := range profiles {
			if profiles[i].ID == envelope.ProfileID {
				selected = &profiles[i]
			}
		}
		if selected == nil || selected.ConfigDigest != envelope.ConfigDigest || selected.verify() != nil {
			held()
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 45*time.Second)
		defer cancel()
		c := command(ctx, *selected, "--config", selected.ConfigRef)
		c.Stdin = bytes.NewReader(envelope.Call)
		var out limitedOutput
		c.Stdout = writerOnly{&out}
		c.Stderr = io.Discard
		if c.Run() != nil || out.overflow || out.Len() == 0 {
			held()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(out.Bytes())
	})
}
func runReceiverRPC(ctx context.Context, cfg Config) error {
	r := cfg.Remote
	if r == nil || r.verify() != nil {
		return ErrHeld
	}
	listener, e := net.Listen("tcp", r.Listen)
	if e != nil {
		return e
	}
	server := &http.Server{Handler: receiverRPCHandler(cfg.Profiles, *r), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 4096, BaseContext: func(net.Listener) context.Context { return ctx }}
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	defer listener.Close()
	e = server.Serve(listener)
	if ctx.Err() != nil {
		return nil
	}
	return e
}

// ReceiverRPC transports one existing call. An error is an unknown outcome, not
// permission to retry start/ACK; inspect the original identity first.
func ReceiverRPC(ctx context.Context, config string, input io.Reader, output io.Writer) error {
	if private(filepath.Dir(config), true) != nil {
		return ErrHeld
	}
	b, e := readPrivate(config)
	var cfg ReceiverRPCClient
	if e != nil || strict(b, &cfg) != nil || cfg.Version != ReceiverRPCClientVersion || !tailAddress(cfg.Address) || cfg.ServerNodeID == "" || cfg.ServerUserID == "" || cfg.ProfileID == "" || !digest(cfg.ConfigDigest) || rpcRuntime(cfg.Tailscale, cfg.Runtime) != nil {
		return ErrHeld
	}
	call, e := boundedRead(input)
	if e != nil || receiverCall(call) != nil {
		return ErrHeld
	}
	b, e = json.Marshal(receiverEnvelope{ReceiverRPCVersion, cfg.ProfileID, cfg.ConfigDigest, call})
	if e != nil || len(b) > maxFrame {
		return ErrHeld
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != cfg.Address {
			return nil, ErrHeld
		}
		c, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
		if e != nil {
			return nil, e
		}
		if rpcWhoIs(ctx, cfg.Tailscale, cfg.Runtime, c.RemoteAddr().String(), cfg.ServerNodeID, cfg.ServerUserID) != nil {
			c.Close()
			return nil, ErrHeld
		}
		return c, nil
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrHeld }}
	req, e := http.NewRequestWithContext(ctx, "POST", "http://"+cfg.Address+"/native-receiver/v1", bytes.NewReader(b))
	if e != nil {
		return ErrHeld
	}
	reply, e := client.Do(req)
	if e != nil {
		return ErrHeld
	}
	defer reply.Body.Close()
	b, e = boundedRead(reply.Body)
	if e != nil || reply.StatusCode != http.StatusOK {
		return ErrHeld
	}
	_, e = output.Write(b)
	return e
}
