package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hev/kit/internal/index"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	var namespace, after string
	var days, limit int
	cmd := &cobra.Command{Use: "outcomes", Short: "Sweep a bounded page of archived session outcomes", RunE: func(cmd *cobra.Command, args []string) error {
		if days < 1 || days > 36500 {
			return fmt.Errorf("days must be 1..36500")
		}
		cl, e := client(namespace)
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()
		next, n, e := index.SweepOutcomes(ctx, cl, index.NewOutcomeEnricher(), after, time.Now().Add(-time.Duration(days)*24*time.Hour).UnixMilli(), limit)
		result := map[string]any{"next": next, "updated": n}
		if e != nil {
			result["error"] = e.Error()
		}
		if encodeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(result); encodeErr != nil {
			return encodeErr
		}
		return e
	}}
	cmd.Flags().StringVar(&namespace, "namespace", "", "archive namespace")
	cmd.Flags().StringVar(&after, "after", "", "ID cursor returned by previous sweep")
	cmd.Flags().IntVar(&days, "days", 60, "lookback days")
	cmd.Flags().IntVar(&limit, "limit", 10, "bounded page size (1..1000)")
	rootCmd.AddCommand(cmd)
}
