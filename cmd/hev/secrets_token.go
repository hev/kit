package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hev/kit/internal/secrets"
	"github.com/spf13/cobra"
)

func secretsChildBaseEnv(clean bool, pass []string) []string {
	if !clean {
		return os.Environ()
	}
	env := []string{}
	for _, name := range pass {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func tokenStoreCommand() *cobra.Command {
	c := &cobra.Command{Use: "token-store", Short: "Create or inspect a Windows DPAPI token store"}
	var path, scope, entropy string
	config := func() secrets.ProtectedTokenStore {
		s := secrets.TokenStoreConfig()
		if path != "" {
			s.Path = path
		}
		if scope != "" {
			s.Scope = scope
		}
		if entropy != "" {
			s.Entropy = []byte(entropy)
		}
		return s
	}
	c.PersistentFlags().StringVar(&path, "file", "", "protected token path (default HEV_OP_DPAPI_FILE)")
	c.PersistentFlags().StringVar(&scope, "scope", "", "CurrentUser or LocalMachine (default HEV_OP_DPAPI_SCOPE or CurrentUser)")
	c.PersistentFlags().StringVar(&entropy, "entropy", "", "optional fixed UTF-8 entropy (default HEV_OP_DPAPI_ENTROPY; not the token)")
	show := func(cmd *cobra.Command) error {
		s := config()
		readers, err := s.Inspect()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "DPAPI token store: %s\nscope: %s\nreaders (SIDs): %s\n", s.Path, s.Scope, strings.Join(readers, ", "))
		return err
	}
	c.AddCommand(&cobra.Command{Use: "inspect", Short: "Report path and ACL readers without decrypting", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return show(cmd) }})
	c.AddCommand(&cobra.Command{Use: "create", Short: "Protect a token from standard input; refuse existing files", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		b, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 65537))
		if err != nil {
			return fmt.Errorf("cannot read token from stdin")
		}
		defer clear(b)
		if len(b) > 65536 {
			return fmt.Errorf("token exceeds 64 KiB")
		}
		if err := config().Write(b); err != nil {
			return err
		}
		return show(cmd)
	}})
	return c
}
