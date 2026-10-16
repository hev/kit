package main

import (
	"encoding/json"
	"errors"
	"github.com/hev/kit/pkg/nodeinventory"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{Use: "_resource-observe node", Hidden: true, SilenceErrors: true, SilenceUsage: true}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 || args[0] != "node" {
			return errors.New(`{"reason":"InvalidArgument","message":"_resource-observe node"}`)
		}
		obs, err := nodeinventory.Observe(cmd.Context())
		if err != nil {
			return errors.New(`{"reason":"SourceUnavailable","message":"local OS observation unavailable"}`)
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(obs)
	}
	rootCmd.AddCommand(cmd)
}
