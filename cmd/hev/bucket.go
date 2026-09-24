package main

import (
	"fmt"

	"github.com/hev/kit/internal/daemon"
	"github.com/spf13/cobra"
)

var bucketCmd = &cobra.Command{
	Use:   "bucket",
	Short: "Choose the active archive bucket",
}

var bucketListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured bucket profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := daemon.LoadConfig()
		if err != nil {
			return err
		}
		for _, bucket := range cfg.Buckets {
			marker := " "
			if bucket.Name == cfg.ActiveBucket {
				marker = "*"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\t%s\t%s\n", marker, bucket.Name, bucket.Bucket, bucket.Endpoint)
		}
		return nil
	},
}

var bucketUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the active bucket profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := daemon.SetActiveBucket(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "active bucket: %s\nconfig: %s\n", args[0], path)
		return nil
	},
}

func init() {
	bucketCmd.AddCommand(bucketListCmd)
	bucketCmd.AddCommand(bucketUseCmd)
}
