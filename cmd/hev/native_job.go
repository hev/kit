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
	rootCmd.AddCommand(cmd)
}
