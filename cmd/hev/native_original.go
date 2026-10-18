package main

import (
	"encoding/json"
	"errors"
	"github.com/hev/kit/pkg/nativejob"
	"github.com/spf13/cobra"
)

func newOriginalInspectCommand() *cobra.Command {
	var root, host, executor, job string
	cmd := &cobra.Command{Use: "inspect-original", Short: "Read original enrollment/job metadata without initializing or recovering a journal", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if root == "" || host == "" || executor == "" {
			return errors.New("explicit --root, --host and --executor required")
		}
		proof, e := nativejob.ReadOnlyProof(cmd.Context(), root, nativejob.Enrollment{HostID: host, ExecutorID: executor}, job)
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(proof)
	}}
	cmd.Flags().StringVar(&root, "root", "", "Existing absolute private original journal")
	cmd.Flags().StringVar(&host, "host", "", "Exact original enrolled host")
	cmd.Flags().StringVar(&executor, "executor", "", "Exact original enrolled executor")
	cmd.Flags().StringVar(&job, "job", "", "Exact original job ID; omitted observes enrollment only")
	return cmd
}
