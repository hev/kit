package main

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/pkg/nativehost"
	"github.com/spf13/cobra"
)

func init() {
	host := &cobra.Command{Use: "native-host", Short: "Inspect or contact the explicitly configured native host"}
	var config string
	verify := &cobra.Command{Use: "verify", Args: cobra.NoArgs, Short: "Read-only validation of native host configuration and pins", RunE: func(cmd *cobra.Command, args []string) error {
		c, e := nativehost.VerifyConfig(config)
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Version  string `json:"version"`
			Profiles int    `json:"profiles"`
		}{c.Version, len(c.Profiles)})
	}}
	verify.Flags().StringVar(&config, "config", "", "Absolute private host configuration")
	_ = verify.MarkFlagRequired("config")
	var hostConfig, receiverConfig, descriptor string
	dispatch := &cobra.Command{Use: "dispatch", Args: cobra.NoArgs, Short: "Dispatch one sealed descriptor to its configured host", RunE: func(cmd *cobra.Command, args []string) error {
		if e := nativehost.DispatchDescriptor(cmd.Context(), hostConfig, receiverConfig, descriptor); e != nil {
			return e
		}
		_, e := fmt.Fprintln(cmd.OutOrStdout(), "accepted")
		return e
	}}
	dispatch.Flags().StringVar(&hostConfig, "host-config", "", "Fixed absolute owner host configuration")
	dispatch.Flags().StringVar(&receiverConfig, "config", "", "Exact receiver configuration")
	dispatch.Flags().StringVar(&descriptor, "descriptor", "", "Exact private descriptor")
	for _, f := range []string{"host-config", "config", "descriptor"} {
		_ = dispatch.MarkFlagRequired(f)
	}
	host.AddCommand(verify, dispatch)
	rootCmd.AddCommand(host)
}
