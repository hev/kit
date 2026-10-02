package main

import (
	"context"
	"encoding/json"
	"github.com/hev/kit/internal/index"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	var namespace, after string
	var days, limit int
	cmd := &cobra.Command{Use: "outcomes", Short: "Sweep a bounded page of archived session outcomes", RunE: func(cmd *cobra.Command, args []string) error {
		cl, e := client(namespace)
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()
		next, n, e := index.SweepOutcomes(ctx, cl, index.NewOutcomeEnricher(), after, time.Now().Add(-time.Duration(days)*24*time.Hour).UnixMilli(), limit)
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"next": next, "updated": n})
	}}
	cmd.Flags().StringVar(&namespace, "namespace", "", "archive namespace")
	cmd.Flags().StringVar(&after, "after", "", "ID cursor returned by previous sweep")
	cmd.Flags().IntVar(&days, "days", 60, "lookback days")
	cmd.Flags().IntVar(&limit, "limit", 10, "bounded page size (1..1000)")
	rootCmd.AddCommand(cmd)
}
