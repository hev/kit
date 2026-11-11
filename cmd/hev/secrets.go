package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hev/kit/internal/secrets"
	"github.com/spf13/cobra"
)

// read and env exit exitRateLimited when 1Password (or the daemon's cooldown)
// answered with a rate limit: a caller must not retry before the time printed
// on stderr. With --cache-only they exit exitUnavailable when no local
// endpoint serves this account, so a caller can keep its own fallback, and
// exitRateLimited for any answer from the endpoint, which is final. Other
// failures exit 1.
const (
	exitUnavailable = 69
	exitRateLimited = 75
)

func resolveSecrets(cmd *cobra.Command, rs []secrets.Ref, caller string, cacheOnly bool) (map[string]string, error) {
	values, err := secrets.ResolveWith(cmd.Context(), rs, secrets.Options{Caller: caller, CacheOnly: cacheOnly})
	if err == nil {
		return values, nil
	}
	switch {
	case cacheOnly && errors.Is(err, secrets.ErrUnavailable):
		fmt.Fprintln(cmd.ErrOrStderr(), "hev secrets:", err)
		return nil, &commandExit{code: exitUnavailable}
	case cacheOnly || secrets.IsRateLimit(err):
		fmt.Fprintln(cmd.ErrOrStderr(), "hev secrets:", err)
		return nil, &commandExit{code: exitRateLimited}
	}
	return nil, err
}

// parseSecretRefs turns repeated NAME=op://... and NAME=duration flags into refs.
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
			return nil, fmt.Errorf("invalid reference; use NAME=op://vault/item/field")
		}
		d := 5 * time.Minute
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

// shellQuote single-quotes a value for POSIX sh, bash and zsh eval. Multiline
// values survive; the quoting never interpolates.
func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

func init() {
	c := &cobra.Command{Use: "secrets", Short: "Resolve service-account secrets through the local memory cache"}
	var refs, ttls []string
	var caller string
	run := &cobra.Command{Use: "run --ref NAME=op://vault/item/field -- cmd ...", Short: "Pass secrets only in the child environment", SilenceUsage: true, Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		rs, err := parseSecretRefs(refs, ttls)
		if err != nil {
			return err
		}
		if caller == "" {
			caller = secrets.DefaultCaller("hev secrets run " + args[0])
		}
		values, err := resolveSecrets(cmd, rs, caller, false)
		if err != nil {
			return err
		}
		child := exec.CommandContext(cmd.Context(), args[0], args[1:]...)
		env := []string{}
		for _, s := range os.Environ() {
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
	run.Flags().StringArrayVar(&refs, "ref", nil, "NAME=op://vault/item/field (repeatable)")
	run.Flags().StringArrayVar(&ttls, "ttl", nil, "NAME=duration; 0s requires a fresh read (default 5m, maximum 24h)")
	run.Flags().StringVar(&caller, "caller", "", "name recorded in the host's read log (default: parent and command)")

	// read replaces `x=$(op read REF)`: the value goes to stdout, which a
	// command substitution reads through a pipe. Nothing touches disk and the
	// value is never a process argument.
	var readTTL time.Duration
	var readLabel, readCaller string
	var readCacheOnly bool
	read := &cobra.Command{Use: "read op://vault/item/field", Short: "Print one secret to stdout for x=$(hev secrets read REF)", SilenceUsage: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if readCaller == "" {
			readCaller = secrets.DefaultCaller("hev secrets read")
		}
		values, err := resolveSecrets(cmd, []secrets.Ref{{Name: readLabel, URI: args[0], TTL: readTTL}}, readCaller, readCacheOnly)
		if err != nil {
			return err
		}
		// Like `op read`, end with one newline; $(...) strips it.
		_, err = io.WriteString(cmd.OutOrStdout(), strings.TrimRight(values[readLabel], "\n")+"\n")
		return err
	}}
	read.Flags().DurationVar(&readTTL, "ttl", 5*time.Minute, "freshness; 0s requires a fresh read (maximum 24h)")
	read.Flags().StringVar(&readLabel, "label", "READ", "label recorded in the read log (a variable-style name, never the value)")
	read.Flags().StringVar(&readCaller, "caller", "", "name recorded in the host's read log (default: parent and command)")
	read.Flags().BoolVar(&readCacheOnly, "cache-only", false, "never read directly: exit 69 when no local daemon serves this account, 75 for any daemon error")

	// env replaces a shell file that runs one `op read` per variable: every
	// reference resolves in one batch, and the output is `export NAME='value'`
	// lines for eval "$(hev secrets env ...)" in the calling shell, so the
	// exports land in that shell and not in a pipeline subshell.
	var envRefs, envTTLs []string
	var envCaller, envFormat string
	var envCacheOnly bool
	env := &cobra.Command{Use: "env --ref NAME=op://vault/item/field ...", Short: "Print export lines for eval \"$(hev secrets env ...)\"", SilenceUsage: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		rs, err := parseSecretRefs(envRefs, envTTLs)
		if err != nil {
			return err
		}
		if len(rs) == 0 {
			return fmt.Errorf("at least one --ref is required")
		}
		if envFormat != "sh" && envFormat != "json" {
			return fmt.Errorf("--format must be sh or json")
		}
		if envCaller == "" {
			envCaller = secrets.DefaultCaller("hev secrets env")
		}
		values, err := resolveSecrets(cmd, rs, envCaller, envCacheOnly)
		if err != nil {
			return err
		}
		if envFormat == "json" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(values)
		}
		var b strings.Builder
		for _, r := range rs {
			b.WriteString("export " + r.Name + "=" + shellQuote(values[r.Name]) + "\n")
		}
		_, err = io.WriteString(cmd.OutOrStdout(), b.String())
		return err
	}}
	env.Flags().StringArrayVar(&envRefs, "ref", nil, "NAME=op://vault/item/field (repeatable)")
	env.Flags().StringArrayVar(&envTTLs, "ttl", nil, "NAME=duration; 0s requires a fresh read (default 5m, maximum 24h)")
	env.Flags().StringVar(&envCaller, "caller", "", "name recorded in the host's read log (default: parent and command)")
	env.Flags().StringVar(&envFormat, "format", "sh", "sh (export lines for eval) or json (object for programs)")
	env.Flags().BoolVar(&envCacheOnly, "cache-only", false, "never read directly: exit 69 when no local daemon serves this account, 75 for any daemon error")

	var statusJSON bool
	status := &cobra.Command{Use: "status", Short: "Show cache hits, misses, real 1Password requests and the account quota", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, e := secrets.ReadStatus(cmd.Context())
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
	c.AddCommand(run, read, env, status)
	rootCmd.AddCommand(c)
}

func writeSecretsStatus(w io.Writer, s *secrets.Status) {
	stamp := func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.UTC().Format(time.RFC3339)
	}
	if s.Daemon {
		fmt.Fprintf(w, "daemon:          up since %s\n", stamp(s.StartedAt))
		fmt.Fprintf(w, "cache:           %d hits, %d misses\n", s.Hits, s.Misses)
		fmt.Fprintf(w, "real requests:   %d since start (%d batches last hour, %d last day, %d rate-limited)\n", s.Requests, s.ReadsHour, s.ReadsDay, s.RateLimited)
		if s.LastLimit != "" {
			fmt.Fprintf(w, "cooldown:        %s; no request before %s\n", s.LastLimit, stamp(s.Reset))
		}
	} else {
		fmt.Fprintln(w, "daemon:          unavailable (no local endpoint serves this account; reads go direct)")
	}
	if s.HostReadsDay != nil {
		fmt.Fprintf(w, "host read log:   %d real requests in the last 24h (%s)\n", *s.HostReadsDay, s.ReadLog)
	}
	if len(s.Quota) == 0 {
		fmt.Fprintf(w, "account quota:   unavailable\n")
		return
	}
	fmt.Fprintf(w, "account quota:   checked %s\n", stamp(s.QuotaCheckedAt))
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
