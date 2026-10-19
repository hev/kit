package main

import (
	"encoding/json"
	"errors"

	"github.com/hev/kit/pkg/nativejob"
	"github.com/spf13/cobra"
)

func init() {
	var root, host, executor string
	cmd := &cobra.Command{Use: "native-job", Short: "Run one opt-in local native-jobs/v1 journal call from stdin", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if root == "" || host == "" || executor == "" {
			return errors.New("explicit --root, --host and --executor required")
		}
		call, err := nativejob.DecodeCall(cmd.InOrStdin())
		if err != nil {
			return err
		}
		store, err := nativejob.Open(root, nativejob.Enrollment{HostID: host, ExecutorID: executor})
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.Call(call)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Version string `json:"version"`
			Result  any    `json:"result"`
		}{nativejob.Version, result})
	}}
	cmd.Flags().StringVar(&root, "root", "", "Private absolute journal directory (durable local storage)")
	cmd.Flags().StringVar(&host, "host", "", "Enrolled host identity")
	cmd.Flags().StringVar(&executor, "executor", "", "Enrolled executor identity")
	cmd.AddCommand(newOriginalInspectCommand(), newIdleInspectCommand())
	rootCmd.AddCommand(cmd)
}

func newIdleInspectCommand() *cobra.Command {
	var root, host, executor string
	cmd := &cobra.Command{Use: "inspect-idle", Args: cobra.NoArgs, Short: "Read-only existing journal idle snapshot; never admission", RunE: func(cmd *cobra.Command, args []string) error {
		proof, e := nativejob.ReadOnlyIdle(cmd.Context(), root, nativejob.Enrollment{HostID: host, ExecutorID: executor})
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(proof)
	}}
	cmd.Flags().StringVar(&root, "root", "", "Existing private absolute journal directory")
	cmd.Flags().StringVar(&host, "host", "", "Enrolled host")
	cmd.Flags().StringVar(&executor, "executor", "", "Enrolled executor")
	for _, f := range []string{"root", "host", "executor"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}
