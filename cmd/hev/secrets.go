package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/secrets"
	"github.com/spf13/cobra"
)

// read and env exit exitRateLimited when a provider (1Password, a Key Vault
// 429, or that provider's cooldown) answered with a rate limit: a caller must not retry before the time printed
// on stderr. With --cache-only they exit exitUnavailable when no local
// endpoint serves this account, so a caller can keep its own fallback, and
// exitRateLimited for any answer from the endpoint, which is final. Other
// failures exit 1.
//
// A read with no daemon falls back to a direct request and is never
// silent: stderr gets secrets.FallbackNotice. With --direct-status, read and
// env then exit exitDirect after delivering the value, so a wrapper can tell
// the paths apart by status alone. --explain prints each reference's
// disposition (label only) to stderr.
const (
	exitUnavailable = 69
	exitRateLimited = 75
	exitDirect      = 78
)

// resolveFlags are the shared options of run, read and env.
type resolveFlags struct {
	caller       string
	cacheOnly    bool
	explain      bool
	directStatus bool
}

func (f *resolveFlags) register(c *cobra.Command, withStatus bool) {
	c.Flags().StringVar(&f.caller, "caller", "", "name recorded in the host's read log (default: parent and command)")
	c.Flags().BoolVar(&f.explain, "explain", false, "print each reference's disposition to stderr: NAME source=cache|read via=daemon|direct (labels only, never values)")
	if withStatus {
		c.Flags().BoolVar(&f.cacheOnly, "cache-only", false, "never read directly: exit 69 when no local daemon serves this account, 75 for any daemon error")
		c.Flags().BoolVar(&f.directStatus, "direct-status", false, "exit 78 instead of 0 when the value was delivered by a direct read because no daemon serves this account")
	}
}

func resolveSecrets(cmd *cobra.Command, rs []secrets.Ref, f resolveFlags) (secrets.Result, error) {
	res, err := secrets.ResolveResult(cmd.Context(), rs, secrets.Options{Caller: f.caller, CacheOnly: f.cacheOnly, Notice: cmd.ErrOrStderr()})
	if err == nil {
		if f.explain {
			explainSources(cmd.ErrOrStderr(), rs, res)
		}
		return res, nil
	}
	switch {
	case secrets.IsBudgetError(err):
		fmt.Fprintln(cmd.ErrOrStderr(), "hev secrets:", err)
		return res, &commandExit{code: 76}
	case f.cacheOnly && errors.Is(err, secrets.ErrUnavailable):
		fmt.Fprintln(cmd.ErrOrStderr(), "hev secrets:", err, "at", res.Endpoint)
		return res, &commandExit{code: exitUnavailable}
	case f.cacheOnly || secrets.IsRateLimit(err):
		fmt.Fprintln(cmd.ErrOrStderr(), "hev secrets:", err)
		return res, &commandExit{code: exitRateLimited}
	}
	return res, err
}

// explainSources writes one label-only line per reference.
func explainSources(w io.Writer, rs []secrets.Ref, res secrets.Result) {
	for _, r := range rs {
		src := res.Sources[r.Name]
		if src == "" {
			src = "unknown"
		}
		fmt.Fprintf(w, "hev secrets: %s source=%s via=%s\n", r.Name, src, res.Via)
	}
}

// directExit is the exit after a successful delivery.
func directExit(f resolveFlags, res secrets.Result) error {
	if f.directStatus && res.Via == secrets.ViaDirect {
		return &commandExit{code: exitDirect}
	}
	return nil
}

// parseSecretRefs turns repeated NAME=REF (secret://..., op://... or azkv://...) and
// NAME=duration flags into refs.
func parseSecretRefs(refs, ttls []string) ([]secrets.Ref, error) {
	durations := map[string]time.Duration{}
	for _, s := range ttls {
		k, v, ok := strings.Cut(s, "=")
		d, e := time.ParseDuration(v)
		if !ok || e != nil {
			return nil, fmt.Errorf("invalid TTL; use NAME=duration")
		}
		durations[k] = d
	}
	rs := []secrets.Ref{}
	for _, s := range refs {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("invalid reference; use NAME=secret://name, NAME=op://vault/item/field or NAME=azkv://vault/secret[/version]")
		}
		d := defaultTTL
		if x, ok := durations[k]; ok {
			d = x
			delete(durations, k)
		}
		rs = append(rs, secrets.Ref{Name: k, URI: v, TTL: d})
	}
	if len(durations) > 0 {
		return nil, fmt.Errorf("TTL has no matching reference")
	}
	return rs, nil
}

const (
	refFlagHelp  = "NAME=secret://name (through this host's secret manager), NAME=op://vault/item/field or NAME=azkv://vault/secret[/version] (repeatable; schemes mix; wins over --env-file)"
	envFileHelp  = "read NAME=VALUE lines: secret://, op:// and azkv:// values are resolved, other values pass through, # comments and blank lines are ignored"
	viaNoRequest = "none"
)

// resolveInputs resolves rs, or, when an env file supplied only plain values,
// makes no request at all.
func resolveInputs(cmd *cobra.Command, rs []secrets.Ref, fromFile bool, f resolveFlags) (secrets.Result, error) {
	if len(rs) == 0 && fromFile {
		return secrets.Result{Values: map[string]string{}, Sources: map[string]string{}, Via: viaNoRequest}, nil
	}
	return resolveSecrets(cmd, rs, f)
}

// withPlain adds an env file's plain values to resolved ones.
func withPlain(values, plain map[string]string) map[string]string {
	out := make(map[string]string, len(values)+len(plain))
	for k, v := range plain {
		out[k] = v
	}
	for k, v := range values {
		out[k] = v
	}
	return out
}

// shellQuote single-quotes a value for POSIX sh, bash and zsh eval. Multiline
// values survive; the quoting never interpolates.
func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

var secretsAttributionRPC = dispatch.RPC

func init() {
	c := &cobra.Command{Use: "secrets", Short: "Resolve service-account secrets through the local memory cache"}
	var refs, ttls []string
	var cleanEnv bool
	var passEnv []string
	var runEnvFile string
	var runFlags resolveFlags
	run := &cobra.Command{Use: "run [--env-file FILE] [--ref NAME=REF] -- cmd ...", Short: "Pass secrets only in the child environment", SilenceUsage: true, Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		rs, plain, _, err := secretInputs(runEnvFile, refs, ttls)
		if err != nil {
			return err
		}
		if runFlags.caller == "" {
			runFlags.caller = secrets.DefaultCaller("hev secrets run " + args[0])
		}
		res, err := resolveInputs(cmd, rs, runEnvFile != "", runFlags)
		if err != nil {
			return err
		}
		values := withPlain(res.Values, plain)
		child := exec.CommandContext(cmd.Context(), args[0], args[1:]...)
		env := []string{}
		for _, s := range secretsChildBaseEnv(cleanEnv, passEnv) {
			k, _, _ := strings.Cut(s, "=")
			if _, ok := values[k]; !ok {
				env = append(env, s)
			}
		}
		for k, v := range values {
			env = append(env, k+"="+v)
		}
		child.Env = env
		child.Stdin = cmd.InOrStdin()
		child.Stdout = cmd.OutOrStdout()
		child.Stderr = cmd.ErrOrStderr()
		if err := runSecretsChild(child); err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				return &commandExit{code: e.ExitCode()}
			}
			return fmt.Errorf("could not start secrets child")
		}
		return nil
	}}
	run.Flags().StringArrayVar(&refs, "ref", nil, refFlagHelp)
	run.Flags().StringVar(&runEnvFile, "env-file", "", envFileHelp)
	run.Flags().StringArrayVar(&ttls, "ttl", nil, "NAME=duration; 0s requires a fresh read (default 5m, maximum 24h)")
	runFlags.register(run, false)
	run.Flags().BoolVar(&runFlags.cacheOnly, "cache-only", false, "never read directly: exit 69 when no local daemon serves this account, 75 for any daemon error")
	run.Flags().BoolVar(&cleanEnv, "clean-env", false, "start child with an empty environment plus resolved references and --pass-env names")
	run.Flags().StringArrayVar(&passEnv, "pass-env", nil, "copy a named nonsecret caller variable to the clean child (repeatable)")
	c.AddCommand(tokenStoreCommand())

	// read replaces `x=$(op read REF)`: the value goes to stdout, which a
	// command substitution reads through a pipe. Nothing touches disk and the
	// value is never a process argument.
	var readTTL time.Duration
	var readLabel string
	var readFlags resolveFlags
	read := &cobra.Command{Use: "read REF (secret://name, op://vault/item/field or azkv://vault/secret[/version])", Short: "Print one secret to stdout for x=$(hev secrets read REF)", SilenceUsage: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if readFlags.caller == "" {
			readFlags.caller = secrets.DefaultCaller("hev secrets read")
		}
		res, err := resolveSecrets(cmd, []secrets.Ref{{Name: readLabel, URI: args[0], TTL: readTTL}}, readFlags)
		if err != nil {
			return err
		}
		// Like `op read`, end with one newline; $(...) strips it.
		if _, err = io.WriteString(cmd.OutOrStdout(), strings.TrimRight(res.Values[readLabel], "\n")+"\n"); err != nil {
			return err
		}
		return directExit(readFlags, res)
	}}
	read.Flags().DurationVar(&readTTL, "ttl", 5*time.Minute, "freshness; 0s requires a fresh read (maximum 24h)")
	read.Flags().StringVar(&readLabel, "label", "READ", "label recorded in the read log (a variable-style name, never the value)")
	readFlags.register(read, true)

	// env replaces a shell file that runs one `op read` per variable: every
	// reference resolves in one batch, and the output is `export NAME='value'`
	// lines for eval "$(hev secrets env ...)" in the calling shell, so the
	// exports land in that shell and not in a pipeline subshell.
	var envRefs, envTTLs []string
	var envEnvFile, envFormat string
	var envFlags resolveFlags
	env := &cobra.Command{Use: "env [--env-file FILE] [--ref NAME=REF] ...", Short: "Print export lines for eval \"$(hev secrets env ...)\"", SilenceUsage: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		rs, plain, order, err := secretInputs(envEnvFile, envRefs, envTTLs)
		if err != nil {
			return err
		}
		if len(rs) == 0 && envEnvFile == "" {
			return fmt.Errorf("at least one --ref or --env-file is required")
		}
		if envFormat != "sh" && envFormat != "json" && envFormat != "json-detail" {
			return fmt.Errorf("--format must be sh, json or json-detail")
		}
		if envFlags.caller == "" {
			envFlags.caller = secrets.DefaultCaller("hev secrets env")
		}
		res, err := resolveInputs(cmd, rs, envEnvFile != "", envFlags)
		if err != nil {
			return err
		}
		res.Values = withPlain(res.Values, plain)
		switch envFormat {
		case "json":
			err = json.NewEncoder(cmd.OutOrStdout()).Encode(res.Values)
		case "json-detail":
			err = json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Values   map[string]string `json:"values"`
				Sources  map[string]string `json:"sources"`
				Via      string            `json:"via"`
				Endpoint string            `json:"endpoint"`
			}{res.Values, res.Sources, res.Via, res.Endpoint})
		default:
			var b strings.Builder
			for _, name := range order {
				b.WriteString("export " + name + "=" + shellQuote(res.Values[name]) + "\n")
			}
			_, err = io.WriteString(cmd.OutOrStdout(), b.String())
		}
		if err != nil {
			return err
		}
		return directExit(envFlags, res)
	}}
	env.Flags().StringArrayVar(&envRefs, "ref", nil, refFlagHelp)
	env.Flags().StringVar(&envEnvFile, "env-file", "", envFileHelp)
	env.Flags().StringArrayVar(&envTTLs, "ttl", nil, "NAME=duration; 0s requires a fresh read (default 5m, maximum 24h)")
	env.Flags().StringVar(&envFormat, "format", "sh", "sh (export lines for eval), json (object for programs) or json-detail (values plus sources, via and endpoint)")
	envFlags.register(env, true)

	var statusJSON, statusNoQuota bool
	status := &cobra.Command{Use: "status", Short: "Show each provider (1Password, Azure Key Vault), cache hits, misses, real requests, cooldowns and the 1Password quota", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		read := secrets.ReadStatus
		if statusNoQuota {
			read = secrets.ReadLocalStatus
		}
		s, e := read(cmd.Context())
		if s == nil {
			return e
		}
		if statusJSON {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(s); err != nil {
				return err
			}
			return e
		}
		writeSecretsStatus(cmd.OutOrStdout(), s)
		return e
	}}
	status.Flags().BoolVar(&statusJSON, "json", false, "print JSON")
	status.Flags().BoolVar(&statusNoQuota, "no-quota", false, "skip the account quota check, so status makes no 1Password request (shows the last known rows)")

	// ping answers "is the cache up?" without any 1Password request, on
	// either side: exit 0 when a daemon serves this account, 69 when none does.
	var pingJSON bool
	ping := &cobra.Command{Use: "ping", Short: "Report whether a local secrets daemon serves this account (never reads 1Password)", SilenceUsage: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		err := secrets.Ping(cmd.Context())
		up := err == nil
		if err != nil && !errors.Is(err, secrets.ErrUnavailable) {
			fmt.Fprintln(cmd.ErrOrStderr(), "hev secrets:", err)
		}
		if pingJSON {
			json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"daemon": up, "endpoint": secrets.Endpoint()})
		} else if up {
			fmt.Fprintln(cmd.OutOrStdout(), "up", secrets.Endpoint())
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "unavailable", secrets.Endpoint())
		}
		if !up {
			return &commandExit{code: exitUnavailable}
		}
		return nil
	}}
	ping.Flags().BoolVar(&pingJSON, "json", false, "print JSON")

	var summaryConfig string
	attribution := &cobra.Command{Use: "attribution", Short: "Show secret request spending by host and caller (owner only)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := dispatch.Load(dispatch.ConfigPath(summaryConfig))
		if err != nil {
			return err
		}
		names := []string{}
		for n := range cfg.Hosts {
			names = append(names, n)
		}
		sort.Strings(names)
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		defer w.Flush()
		fmt.Fprintln(w, "HOST\tCALLER\tPROVIDER\tREQUESTS\tCACHE HITS\tTHROTTLES")
		failed := 0
		for _, n := range names {
			r, e := secretsAttributionRPC(cmd.Context(), cfg.Hosts[n], dispatch.Request{Action: "secrets-summary"})
			if e != nil {
				failed++
				fmt.Fprintf(w, "%s\tERROR: %s\n", n, e)
				continue
			}
			for _, s := range r.SecretSummary {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\n", n, s.Caller, s.Provider, s.Requests, s.CacheHits, s.Throttles)
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d hosts could not report secrets attribution", failed)
		}
		return nil
	}}
	attribution.Flags().StringVar(&summaryConfig, "config", "", "Dispatch host configuration")
	c.AddCommand(run, read, env, status, ping, attribution)
	rootCmd.AddCommand(c)
}

// writeManager prints the host's secret manager for secret:// references and
// where it is set, or what to set.
func writeManager(w io.Writer, m *secrets.ManagerStatus) {
	if m == nil {
		return
	}
	if !m.Configured {
		fmt.Fprintf(w, "secret manager:  not set; secret:// cannot resolve\n  setup: %s\n", m.Setup)
		return
	}
	detail := "vault " + m.Vault
	if m.Kind == secrets.ManagerOnePassword {
		detail += ", field " + m.Field
	}
	fmt.Fprintf(w, "secret manager:  %s (%s; %s)\n", m.ManagerName, detail, m.Source)
}

// writeProviderRows prints one row per provider: configured or not, the
// credential type (never a value), reachability from the last real request,
// counters and any cooldown, then each provider's quota note.
func writeProviderRows(w io.Writer, s *secrets.Status, stamp func(time.Time) string) {
	if len(s.Providers) == 0 {
		return
	}
	fmt.Fprintln(w, "providers:")
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "  PROVIDER\tSCHEME\tCREDENTIAL\tREACHABLE\tHITS\tMISSES\tREQUESTS\tCOOLDOWN")
	for _, p := range s.Providers {
		cred := p.Credential
		if !p.Configured {
			cred = "not configured"
		}
		cool := "-"
		if p.LastLimit != "" && time.Now().Before(p.Reset) {
			cool = p.LastLimit + " until " + stamp(p.Reset)
		}
		reqs := fmt.Sprint(p.Requests)
		if p.TokenRequests > 0 {
			reqs += fmt.Sprintf(" (+%d token)", p.TokenRequests)
		}
		fmt.Fprintf(tw, "  %s\t%s://\t%s\t%s\t%d\t%d\t%s\t%s\n", p.Name, p.Scheme, cred, p.Reachable, p.Hits, p.Misses, reqs, cool)
	}
	tw.Flush()
	for _, p := range s.Providers {
		if !p.Configured && p.Setup != "" {
			fmt.Fprintf(w, "  %s setup: %s\n", p.Name, p.Setup)
		}
	}
	for _, p := range s.Providers {
		if p.Quota != "" && p.Scheme != "op" {
			fmt.Fprintf(w, "  %s quota: %s\n", p.Name, p.Quota)
		}
	}
}

func writeSecretsStatus(w io.Writer, s *secrets.Status) {
	stamp := func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(w, "host budget: %d/%d day, %d/%d hour; caller %d/min; throttles %d last=%s; %s\n", s.Budget.UsedDay, s.Budget.Daily, s.Budget.UsedHour, s.Budget.Hourly, s.Budget.CallerMinute, s.Budget.Throttles, s.Budget.LastCaller, s.Budget.Error)
	fmt.Fprintf(w, "stale deliveries: %d; notify: %s\nmeter: %s\n", s.Stale, s.NotifyError, s.MeterNote)
	if s.Daemon {
		fmt.Fprintf(w, "daemon:          up since %s\n", stamp(s.StartedAt))
		fmt.Fprintf(w, "cache:           %d hits, %d misses\n", s.Hits, s.Misses)
		fmt.Fprintf(w, "real requests:   %d since start (%d batches last hour, %d last day, %d rate-limited)\n", s.Requests, s.ReadsHour, s.ReadsDay, s.RateLimited)
	} else {
		fmt.Fprintln(w, "daemon:          unavailable (no local endpoint serves this account; reads go direct)")
	}
	if s.LastLimit != "" && time.Now().Before(s.Reset) {
		fmt.Fprintf(w, "cooldown:        %s; no request from this host before %s (%s)\n", s.LastLimit, stamp(s.Reset), secrets.CooldownPath())
	}
	if s.HostReadsDay != nil {
		fmt.Fprintf(w, "host read log:   %d real requests in the last 24h (%s)\n", *s.HostReadsDay, s.ReadLog)
	}
	writeManager(w, s.Manager)
	writeProviderRows(w, s, stamp)
	if len(s.Quota) == 0 {
		fmt.Fprintf(w, "account quota:   unavailable (%s)\n", s.QuotaUnavailable)
		return
	}
	fmt.Fprintf(w, "account quota:   checked %s (1Password)\n", stamp(s.QuotaCheckedAt))
	if s.QuotaUnavailable != "" {
		fmt.Fprintf(w, "meter warning: %s\n", s.QuotaUnavailable)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "  TYPE\tACTION\tLIMIT\tUSED\tREMAINING\tRESET")
	n := func(p *int64) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprint(*p)
	}
	rows := append([]secrets.QuotaRow(nil), s.Quota...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Type < rows[j].Type })
	for _, r := range rows {
		reset := r.Reset
		if reset == "" {
			reset = "-"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", r.Type, r.Action, n(r.Limit), n(r.Used), n(r.Remaining), reset)
	}
	tw.Flush()
}
