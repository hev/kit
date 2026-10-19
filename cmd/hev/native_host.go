package main

import (
	"encoding/json"
	"fmt"
	"github.com/hev/kit/pkg/nativehost"
	n "github.com/hev/kit/pkg/nativejob"
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
		digests := make(map[string]string, len(c.Profiles))
		for _, p := range c.Profiles {
			digests[p.ID] = n.Digest(p)
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Version        string            `json:"version"`
			Profiles       int               `json:"profiles"`
			ProfileDigests map[string]string `json:"profile_digests"`
		}{c.Version, len(c.Profiles), digests})
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
	observe := &cobra.Command{Use: "observe-coalition", Args: cobra.NoArgs, Short: "Read-only birth-bound kernel coalition observation", RunE: func(cmd *cobra.Command, args []string) error {
		return nativehost.WriteCoalitionObservation(cmd.InOrStdin(), cmd.OutOrStdout())
	}}
	var rpcConfig string
	rpc := &cobra.Command{Use: "receiver-rpc", Args: cobra.NoArgs, Short: "Send one original receiver call to its pinned Tailnet host", RunE: func(cmd *cobra.Command, args []string) error {
		return nativehost.ReceiverRPC(cmd.Context(), rpcConfig, cmd.InOrStdin(), cmd.OutOrStdout())
	}}
	rpc.Flags().StringVar(&rpcConfig, "config", "", "Absolute private RPC client configuration")
	_ = rpc.MarkFlagRequired("config")
	service := &cobra.Command{Use: "observe-service", Args: cobra.NoArgs, Short: "Read-only birth-bound service kernel observation", RunE: func(cmd *cobra.Command, args []string) error {
		return nativehost.WriteServiceObservation(cmd.InOrStdin(), cmd.OutOrStdout())
	}}
	host.AddCommand(verify, dispatch, observe, rpc, service)
	rootCmd.AddCommand(host)
}
