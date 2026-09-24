package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/cost"
	"github.com/spf13/cobra"
)

func init() {
	var dir, output string
	cmd := &cobra.Command{Use: "cost-price", Short: "Price the factory accounting export using dated operator tables", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cost.LoadConfig(filepath.Join(dir, "prices.toml"), filepath.Join(dir, "subscriptions.toml"))
		if err != nil {
			return err
		}
		rows, err := cost.ReadRows(filepath.Join(dir, "sessions.jsonl"))
		if err != nil {
			return err
		}
		if err = c.Price(rows); err != nil {
			return err
		}
		if output == "" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		}
		f, err := os.CreateTemp(filepath.Dir(output), ".priced-*")
		if err != nil {
			return err
		}
		name := f.Name()
		defer os.Remove(name)
		enc := json.NewEncoder(f)
		for _, r := range rows {
			if err = enc.Encode(r); err != nil {
				f.Close()
				return err
			}
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = os.Rename(name, output); err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "priced %d sessions\n", len(rows))
		return nil
	}}
	cmd.Flags().StringVar(&dir, "dir", ".", "Directory containing sessions.jsonl, prices.toml and subscriptions.toml")
	cmd.Flags().StringVar(&output, "output", "", "Atomically write priced JSONL for beat accounting (default: JSON array on stdout)")
	rootCmd.AddCommand(cmd)
}
