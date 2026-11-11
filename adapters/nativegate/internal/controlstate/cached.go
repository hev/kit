package controlstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// BatchResolver resolves several references in one request.
type BatchResolver interface {
	ResolveAll(context.Context, []string) ([]string, error)
}

// `hev secrets env --cache-only` exits exitRefused when the local secrets
// daemon answered with an error (a rate limit included); that is final. Any
// other failure, 69 (no daemon serves this account) or an older hev without
// the command, made no request through the cache and falls back to Direct.
const exitRefused = 75

// CachedResolver reads every reference in one request through the host's
// kit secrets daemon (`hev secrets env --cache-only`), whose memory cache
// serves later jobs within TTL. When hev is missing or no daemon serves the
// account it falls back to Direct, the per-reference `op read` the gate always
// used. A daemon that answered with an error (a rate limit included) is final
// and never retried directly. Values arrive on a pipe and are never logged.
// HEV_SECRETS_CACHE=off, or a PATH without hev (a receiver profile that
// supplies a fixture op, for example), keeps the direct path.
type CachedResolver struct {
	Direct OPResolver
	// Hev is the hev executable; empty looks up "hev" on PATH.
	Hev string
	// TTL is how long the daemon may reuse a value; zero means one hour.
	TTL time.Duration
}

func (r CachedResolver) Resolve(ctx context.Context, ref string) (string, error) {
	v, err := r.ResolveAll(ctx, []string{ref})
	if err != nil {
		return "", err
	}
	return v[0], nil
}

func (r CachedResolver) ResolveAll(ctx context.Context, refs []string) ([]string, error) {
	for _, ref := range refs {
		if !validRef(ref) {
			return nil, errors.New("invalid credential reference")
		}
	}
	hev := r.Hev
	if os.Getenv("HEV_SECRETS_CACHE") == "off" {
		hev = "-"
	} else if hev == "" {
		hev, _ = exec.LookPath("hev")
	}
	if hev != "" && hev != "-" {
		vals, err := r.viaDaemon(ctx, hev, refs)
		if !errors.Is(err, errDaemonUnavailable) {
			return vals, err
		}
	}
	// The fallback is never silent; the prefix matches kit's secrets.FallbackNotice.
	fmt.Fprintf(os.Stderr, "hev secrets: notice: fallback to direct 1Password read (native-gate: %d references; no secrets daemon or hev)\n", len(refs))
	out := make([]string, len(refs))
	for i, ref := range refs {
		v, err := r.Direct.Resolve(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

var errDaemonUnavailable = errors.New("secrets daemon unavailable")

func (r CachedResolver) viaDaemon(ctx context.Context, hev string, refs []string) ([]string, error) {
	ttl := r.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	args := []string{"secrets", "env", "--format", "json", "--cache-only", "--caller", "native-gate"}
	for i, ref := range refs {
		name := "NATIVE_GATE_REF_" + strconv.Itoa(i)
		args = append(args, "--ref", name+"="+ref, "--ttl", name+"="+ttl.String())
	}
	cmd := exec.CommandContext(ctx, hev, args...)
	cmd.Env = append(os.Environ(), "OP_BIOMETRIC_UNLOCK_ENABLED=false")
	if r.Direct.TokenFile != "" && os.Getenv("OP_SERVICE_ACCOUNT_TOKEN") == "" {
		cmd.Env = append(cmd.Env, "HEV_OP_TOKEN_FILE="+filepath.Clean(r.Direct.TokenFile))
	}
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == exitRefused {
			return nil, errors.New("credential resolution failed")
		}
		return nil, errDaemonUnavailable
	}
	var values map[string]string
	if json.Unmarshal(out, &values) != nil {
		return nil, errors.New("credential resolution failed")
	}
	vals := make([]string, len(refs))
	for i := range refs {
		vals[i] = values["NATIVE_GATE_REF_"+strconv.Itoa(i)]
		if vals[i] == "" {
			return nil, errors.New("credential resolution failed")
		}
	}
	return vals, nil
}

var readLogMu sync.Mutex

// logDirectRead appends one line to the host's 1Password read log in kit's
// format (see kit's internal/secrets): time, caller, kind, label, outcome.
// The label is a hash prefix of the reference; no value or URI is written.
func logDirectRead(ref, outcome string) {
	p := os.Getenv("HEV_SECRETS_READ_LOG")
	if p == "off" {
		return
	}
	if p == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		p = filepath.Join(home, ".hev", "secrets", "op-reads.log")
	}
	readLogMu.Lock()
	defer readLogMu.Unlock()
	if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	sum := sha256.Sum256([]byte(ref))
	fmt.Fprintf(f, "%s caller=native-gate_direct kind=read refs=1 labels=REF#%x outcome=%s\n", time.Now().UTC().Format(time.RFC3339), sum[:4], outcome)
}
