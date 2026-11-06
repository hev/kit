package main

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/secrets"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"strings"
	"time"
)

func init() {
	c := &cobra.Command{Use: "secrets", Short: "Resolve service-account secrets through the local memory cache"}
	var refs, ttls []string
	run := &cobra.Command{Use: "run --ref NAME=op://vault/item/field -- cmd ...", Short: "Pass secrets only in the child environment", SilenceUsage: true, Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		durations := map[string]time.Duration{}
		for _, s := range ttls {
			k, v, ok := strings.Cut(s, "=")
			d, e := time.ParseDuration(v)
			if !ok || e != nil {
				return fmt.Errorf("invalid TTL; use NAME=duration")
			}
			durations[k] = d
		}
		rs := []secrets.Ref{}
		for _, s := range refs {
			k, v, ok := strings.Cut(s, "=")
			if !ok {
				return fmt.Errorf("invalid reference; use NAME=op://vault/item/field")
			}
			d := 5 * time.Minute
			if x, ok := durations[k]; ok {
				d = x
				delete(durations, k)
			}
			rs = append(rs, secrets.Ref{Name: k, URI: v, TTL: d})
		}
		if len(durations) > 0 {
			return fmt.Errorf("TTL has no matching reference")
		}
		values, err := secrets.Resolve(cmd.Context(), rs)
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
	c.AddCommand(run, &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, e := secrets.ReadStatus(cmd.Context())
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(s)
	}})
	rootCmd.AddCommand(c)
}
