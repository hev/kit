package secrets

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

type request struct {
	// Account is the 1Password fingerprint, which a daemon that predates
	// providers still checks; Accounts has every configured provider's.
	Account  string
	Accounts map[string]string `json:",omitempty"`
	Refs     []Ref
	Status   bool
	// NoQuota asks for status without the quota check, so no 1Password request.
	NoQuota bool `json:",omitempty"`
	// Ping asks only whether this endpoint serves the account; never a request.
	// A daemon that predates Ping reads it as an empty resolution, also free.
	Ping bool `json:",omitempty"`
	// Caller names the requesting process in the read log only.
	Caller string `json:",omitempty"`
	// WaitTicket preserves a single-reference caller's place during retries.
	WaitTicket string `json:",omitempty"`
}
type response struct {
	Values      map[string]string
	Sources     map[string]string `json:",omitempty"`
	Pong        bool              `json:",omitempty"`
	Status      *Status
	Unavailable bool
	// Serves names the provider schemes (never fingerprints) the endpoint
	// serves, sent with Unavailable so a client can say why it was refused.
	// A daemon that predates it leaves it empty.
	Serves  []string `json:",omitempty"`
	Error   string
	RetryAt *time.Time `json:",omitempty"`
}

func Serve(ctx context.Context, c *Cache) (func(), error) {
	l, err := listen()
	if err != nil {
		return nil, err
	}
	accounts := map[string]string{}
	var serves []string
	for _, p := range c.providers {
		if fp := p.Fingerprint(); fp != "" {
			accounts[p.Scheme()] = fp
			serves = append(serves, p.Scheme())
		}
	}
	done := make(chan struct{})
	monitorCtx, cancelMonitor := context.WithCancel(ctx)
	monitorStopped := make(chan struct{})
	go func() {
		defer close(monitorStopped)
		tick := time.NewTicker(c.monitorEvery)
		defer tick.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-done:
				return
			case <-tick.C:
				check, cancel := context.WithTimeout(monitorCtx, 30*time.Second)
				c.scheduledMeter(check)
				cancel()
			}
		}
	}()

	go func() {
		select {
		case <-ctx.Done():
			l.Close()
		case <-done:
		}
	}()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go serveConn(ctx, c, conn, accounts, serves)
		}
	}()
	return func() { close(done); cancelMonitor(); l.Close(); <-monitorStopped }, nil
}

// served reports whether the endpoint serves this request's accounts: a
// resolution needs a matching fingerprint for every provider it names; ping
// and status need one for any provider the caller has configured.
func served(q request, accounts map[string]string) bool {
	theirs := q.Accounts
	if theirs == nil { // a client that predates providers knows only 1Password
		theirs = map[string]string{"op": q.Account}
	}
	match := func(s string) bool { return accounts[s] != "" && theirs[s] == accounts[s] }
	if len(q.Refs) == 0 { // ping, status, or an old client's empty resolution
		for s := range theirs {
			if match(s) {
				return true
			}
		}
		return false
	}
	for _, r := range q.Refs {
		if !match(schemeOf(r.URI)) {
			return false
		}
	}
	return true
}

func serveConn(ctx context.Context, c *Cache, conn net.Conn, accounts map[string]string, serves []string) {
	defer conn.Close()
	if !sameUser(conn) {
		return
	}
	deadline := time.Now().Add(45 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var q request
	if json.NewDecoder(io.LimitReader(conn, 65536)).Decode(&q) != nil {
		return
	}
	r := response{}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if !served(q, accounts) {
		r.Unavailable, r.Serves = true, serves
	} else if q.Ping {
		r.Pong = true
	} else if q.Status {
		var s Status
		if q.NoQuota {
			s = c.LocalStatus()
		} else {
			s = c.Status(ctx)
		}
		s.Daemon = true
		r.Status = &s
	} else {
		if len(q.Refs) == 1 && validBudgetTicket(q.WaitTicket) {
			ctx = context.WithValue(ctx, budgetTicketKey{}, q.WaitTicket)
		}
		var err error
		r.Values, r.Sources, err = c.ResolveSources(ctx, q.Caller, q.Refs)
		if err != nil {
			r.Error = err.Error()
			var throttle *callerThrottle
			if errors.As(err, &throttle) {
				r.RetryAt = &throttle.retryAt
			}
		}
	}
	json.NewEncoder(conn).Encode(r)
}

var ErrUnavailable = errors.New("local secrets daemon unavailable")

// Why no local endpoint answered a caller (UnavailableError.Reason).
const (
	// UnavailableNoEndpoint: nothing listens at the endpoint (or it is
	// invalid, or HEV_SECRETS_CACHE=off).
	UnavailableNoEndpoint = "no-endpoint"
	// UnavailableNoCredential: a daemon is up, but this caller has no
	// credential for a provider it serves, so it cannot prove the account.
	UnavailableNoCredential = "no-credential"
	// UnavailableOtherAccount: a daemon is up, but this caller's credential
	// is for a different account than the daemon's.
	UnavailableOtherAccount = "other-account"
)

// UnavailableError is ErrUnavailable with the reason: errors.Is(err,
// ErrUnavailable) holds for it, and its text is ErrUnavailable's.
type UnavailableError struct {
	Reason string
	// Providers are the schemes the reason is about: for no-credential, the
	// ones the daemon serves (1Password when an older daemon does not say);
	// for other-account, the ones both sides have a credential for.
	Providers []string
}

func (e *UnavailableError) Error() string        { return ErrUnavailable.Error() }
func (e *UnavailableError) Is(target error) bool { return target == ErrUnavailable }

// UnavailableReason is the reason in err, or "" when err is not
// ErrUnavailable. A bare ErrUnavailable is no-endpoint.
func UnavailableReason(err error) (reason string, providers []string) {
	var u *UnavailableError
	if errors.As(err, &u) {
		return u.Reason, u.Providers
	}
	if errors.Is(err, ErrUnavailable) {
		return UnavailableNoEndpoint, nil
	}
	return "", nil
}

// unavailableFor explains a refusal from an endpoint that is up. mine are
// this caller's fingerprints by scheme, serves the schemes the daemon said it
// serves (nil from a daemon that predates saying so), need the schemes the
// request was about (nil for ping and status: any).
func unavailableFor(mine map[string]string, serves, need []string) *UnavailableError {
	if serves == nil {
		serves = []string{"op"} // every daemon that predates Serves serves 1Password
	}
	relevant := serves
	if need != nil {
		relevant = need
	}
	var missing, both []string
	for _, s := range relevant {
		if mine[s] == "" {
			missing = append(missing, s)
		} else {
			both = append(both, s)
		}
	}
	if len(both) == 0 || (need != nil && len(missing) > 0) {
		return &UnavailableError{Reason: UnavailableNoCredential, Providers: missing}
	}
	return &UnavailableError{Reason: UnavailableOtherAccount, Providers: both}
}

// CredentialSettings names the settings that give a caller a credential for
// a provider scheme: names only, never a value.
func CredentialSettings(scheme string) string {
	switch scheme {
	case "op":
		return "OP_SERVICE_ACCOUNT_TOKEN, HEV_OP_TOKEN_FILE, or on Windows HEV_OP_DPAPI_FILE with HEV_OP_DPAPI_SCOPE and HEV_OP_DPAPI_ENTROPY"
	case azkvScheme:
		return "HEV_AZKV_CREDENTIAL and the AZURE_* settings it names"
	}
	return ""
}

// ProviderName is a scheme's display name ("1Password"), or the scheme.
func ProviderName(scheme string) string {
	for _, r := range registry {
		if r.scheme == scheme {
			return r.name
		}
	}
	return scheme
}

// DescribeUnavailable says, in one line, why no local endpoint answered:
// never a value or fingerprint.
func DescribeUnavailable(err error, endpoint string) string {
	reason, providers := UnavailableReason(err)
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = ProviderName(p)
	}
	list := strings.Join(names, " and ")
	switch reason {
	case UnavailableNoCredential:
		return fmt.Sprintf("a daemon is up at %s, but this caller has no %s credential, so it does not serve this caller", endpoint, list)
	case UnavailableOtherAccount:
		return fmt.Sprintf("a daemon is up at %s, but it does not serve the %s account this caller's credential is for", endpoint, list)
	}
	return "no daemon serves this account at " + endpoint
}

// EndpointEnv overrides the local endpoint for server and client alike: a
// Unix socket path on Linux and macOS, a \\.\pipe\NAME pipe on Windows. A test or
// a second daemon can run on its own isolated endpoint. Unset keeps the
// default (~/.hev/secrets/agent.sock, or \\.\pipe\hev-secrets-<SID>).
const EndpointEnv = "HEV_SECRETS_ENDPOINT"

// Endpoint is the endpoint this process serves or dials.
func Endpoint() string {
	if v := os.Getenv(EndpointEnv); v != "" {
		return v
	}
	return defaultEndpoint()
}

func endpoint() string { return Endpoint() }

// Ping reports whether a local endpoint serves this account: nil when one
// does, ErrUnavailable when none does. It never makes a 1Password request,
// on either side.
func Ping(ctx context.Context) error {
	if CacheDisabled() {
		return &UnavailableError{Reason: UnavailableNoEndpoint}
	}
	_, err := query(ctx, request{Ping: true})
	return err
}

// query sends q to the local endpoint. When nothing answers it for this
// caller the error is an *UnavailableError saying why. A caller with no
// credential at all sends only a ping, which every daemon refuses without
// a request, to learn whether an endpoint is up.
func query(ctx context.Context, q request) (response, error) {
	q.Account, q.Accounts = accountIDs()
	if validEndpoint(Endpoint()) != nil {
		return response{}, &UnavailableError{Reason: UnavailableNoEndpoint}
	}
	var need []string
	for _, r := range q.Refs {
		if s := schemeOf(r.URI); !slices.Contains(need, s) {
			need = append(need, s)
		}
	}
	mine := q.Accounts
	if len(mine) == 0 {
		q = request{Ping: true}
	}
	conn, err := dial(ctx)
	if err != nil {
		return response{}, &UnavailableError{Reason: UnavailableNoEndpoint}
	}
	defer conn.Close()
	deadline := time.Now().Add(45 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if !serverUser(conn) {
		return response{}, errors.New("secrets endpoint is not owned by current user")
	}
	if err = json.NewEncoder(conn).Encode(q); err != nil {
		return response{}, errors.New("secrets request failed")
	}
	var r response
	if json.NewDecoder(io.LimitReader(conn, 4<<20)).Decode(&r) != nil {
		return r, errors.New("invalid secrets response")
	}
	if r.Unavailable || len(mine) == 0 {
		return r, unavailableFor(mine, r.Serves, need)
	}
	if r.Error != "" {
		if r.Error == callerThrottleMessage && r.RetryAt != nil {
			return r, &callerThrottle{retryAt: *r.RetryAt}
		}
		return r, errors.New(r.Error)
	}
	return r, nil
}
func Resolve(ctx context.Context, refs []Ref) (map[string]string, error) {
	return ResolveWith(ctx, refs, Options{})
}

// CacheDisabled reports HEV_SECRETS_CACHE=off: clients skip the local
// endpoint and behave as if no daemon ran (for a process whose PATH supplies a
// fixture op, for example).
func CacheDisabled() bool { return os.Getenv("HEV_SECRETS_CACHE") == "off" }

// IsRateLimit reports whether err is a rate-limit answer from any provider
// (1Password, a Key Vault 429) or from that provider's cooldown, locally or
// through the endpoint, wrapped or not.
func IsRateLimit(err error) bool {
	return err != nil && strings.Contains(err.Error(), "rate limit; retry after ")
}

// Options adjusts a client resolution.
type Options struct {
	// Caller names this process in the read log; empty uses DefaultCaller.
	Caller string
	// CacheOnly returns ErrUnavailable instead of resolving directly when no
	// endpoint serves this account, so the caller can keep its own fallback.
	CacheOnly bool
	// Notice receives the one-line fallback notice when the read goes direct;
	// nil means os.Stderr. Use io.Discard to silence it deliberately.
	Notice io.Writer
	// CallerWait bounds resolution of a single reference, including time
	// queued for the caller's rolling-minute budget. Other failures return
	// immediately. Zero keeps the ordinary fail-fast behavior.
	CallerWait time.Duration
}

// Via values: how a resolution reached 1Password.
const (
	ViaDaemon = "daemon" // the local endpoint answered
	ViaDirect = "direct" // no endpoint served this account; this process ran op
)

// Result is a resolution with its provenance. Sources maps each name to
// SourceCache or SourceRead; neither it nor Via ever holds a value.
type Result struct {
	Values   map[string]string
	Sources  map[string]string
	Via      string
	Endpoint string
}

// FallbackPrefix starts the stderr line printed when a read falls back to a
// direct request. Wrappers can match it whatever the store.
const FallbackPrefix = "hev secrets: notice: fallback to direct"

// FallbackNotice is that line for a request of 1Password references only, the
// same text kit has always printed.
const FallbackNotice = FallbackPrefix + " 1Password read"

// fallbackNotice names the stores a direct read of refs will ask, in provider
// order: "... direct 1Password read", "... direct Azure Key Vault read" or
// "... direct 1Password and Azure Key Vault read".
func fallbackNotice(refs []Ref) string {
	var names []string
	for _, s := range registry {
		for _, r := range refs {
			if schemeOf(r.URI) == s.scheme {
				names = append(names, s.name)
				break
			}
		}
	}
	return FallbackPrefix + " " + strings.Join(names, " and ") + " read"
}

// ResolveWith asks the local endpoint and, unless opts.CacheOnly, resolves
// directly when no endpoint serves this account. A connected endpoint's error
// (a rate limit included) is returned as is and never retried directly.
func ResolveWith(ctx context.Context, refs []Ref, opts Options) (map[string]string, error) {
	r, err := ResolveResult(ctx, refs, opts)
	return r.Values, err
}

// ResolveResult is ResolveWith with per-reference dispositions and the path
// taken. A direct fallback is never silent: it writes FallbackNotice to
// opts.Notice and returns Via == ViaDirect.
func ResolveResult(ctx context.Context, refs []Ref, opts Options) (Result, error) {
	res := Result{Endpoint: Endpoint()}
	// The caller expands secret:// through its host's manager, so the daemon
	// (of any age) only ever receives op:// or azkv://.
	refs, err := ExpandRefs(refs)
	if err != nil {
		return res, err
	}
	if err := Validate(refs); err != nil {
		return res, err
	}
	if opts.Caller == "" {
		opts.Caller = DefaultCaller("hev")
	}
	q := request{Refs: refs, Caller: opts.Caller}
	if opts.CallerWait > 0 {
		if len(refs) != 1 {
			return res, errors.New("caller budget wait requires a single credential reference")
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.CallerWait)
		defer cancel()
		q.WaitTicket = rand.Text()
		ctx = context.WithValue(ctx, budgetTicketKey{}, q.WaitTicket)
	}
	var r response
	err = ErrUnavailable
	reason := "HEV_SECRETS_CACHE=off"
	if !CacheDisabled() {
		r, err = query(ctx, q)
		reason = DescribeUnavailable(err, res.Endpoint)
	}
	if errors.Is(err, ErrUnavailable) {
		if opts.CacheOnly {
			return res, err
		}
		w := opts.Notice
		if w == nil {
			w = os.Stderr
		}
		fmt.Fprintf(w, "%s (%s)\n", fallbackNotice(refs), reason)
		res.Via = ViaDirect
		for {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			res.Values, res.Sources, err = New().ResolveSources(ctx, opts.Caller+" direct", refs)
			if retry, result := retryCallerBudget(ctx, opts, err); !retry {
				if result != nil {
					res.Values, res.Sources = nil, nil
				}
				return res, result
			}
		}
	}
	res.Via = ViaDaemon
	for {
		retry, result := retryCallerBudget(ctx, opts, err)
		if !retry {
			err = result
			break
		}
		// Once the daemon answers, retries stay on that path.
		r, err = query(ctx, q)
	}
	if err != nil {
		return res, err
	}
	res.Values, res.Sources = r.Values, r.Sources
	if res.Sources == nil { // a daemon that predates dispositions
		res.Sources = map[string]string{}
	}
	return res, nil
}

func validBudgetTicket(ticket string) bool {
	if len(ticket) < 26 || len(ticket) > 64 {
		return false
	}
	for _, ch := range ticket {
		if (ch < 'A' || ch > 'Z') && (ch < '2' || ch > '7') {
			return false
		}
	}
	return true
}

func retryCallerBudget(ctx context.Context, opts Options, err error) (bool, error) {
	if opts.CallerWait <= 0 {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var throttle *callerThrottle
	var delay time.Duration
	switch {
	case errors.As(err, &throttle):
		delay = max(time.Until(throttle.retryAt), 20*time.Millisecond)
	case err != nil && err.Error() == callerThrottleMessage:
		// Older daemons preserve the error but don't send the window's end.
		delay = time.Minute
	default:
		return false, err
	}
	err = waitForBudget(ctx, delay)
	return err == nil, err
}

// ReadStatus asks the endpoint for its counters and adds the host read log's
// 24-hour count. With no endpoint it still returns that count, with
// Daemon false, alongside ErrUnavailable.
func ReadStatus(ctx context.Context) (*Status, error) { return readStatus(ctx, false) }

// ReadLocalStatus is ReadStatus without the quota check: no 1Password request
// on either side. The quota rows are the daemon's last known ones.
func ReadLocalStatus(ctx context.Context) (*Status, error) { return readStatus(ctx, true) }

func readStatus(ctx context.Context, noQuota bool) (*Status, error) {
	r, err := query(ctx, request{Status: true, NoQuota: noQuota})
	s := r.Status
	if s == nil {
		if !errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		// No daemon: each provider's configuration and persisted cooldown,
		// from the environment and files only.
		local := New().LocalStatus()
		s = &local
		s.DaemonUnavailable, s.DaemonUnavailableProviders = UnavailableReason(err)
	}
	// secret:// is expanded by the caller, so the caller's setting is the one
	// that applies.
	s.Manager = HostManagerStatus()
	// The host's persisted cooldown applies whether or not a daemon runs.
	if cd, ok := ActiveCooldown(time.Now()); ok && cd.Reset.After(s.Reset) {
		s.LastLimit, s.Reset = cd.Limit, cd.Reset
	}
	if n, e := CountReadLog(time.Now().Add(-24 * time.Hour)); e == nil {
		s.HostReadsDay = &n
	}
	return s, err
}

// Hold the endpoint lock for the full listener lifetime, including shutdown.
type lockedListener struct {
	net.Listener
	once    sync.Once
	release func()
}

func (l *lockedListener) Close() error { err := l.Listener.Close(); l.once.Do(l.release); return err }

// accountIDs returns the 1Password fingerprint (for daemons that predate
// providers) and every configured provider's fingerprint by scheme.
func accountID() string { id, _ := accountIDs(); return id }

func accountIDs() (string, map[string]string) {
	all := map[string]string{}
	for _, p := range New().providers {
		if fp := p.Fingerprint(); fp != "" {
			all[p.Scheme()] = fp
		}
	}
	return all["op"], all
}
