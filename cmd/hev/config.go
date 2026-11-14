package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

var (
	configInitForce bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage daemon configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a starter config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := daemon.WriteDefaultConfig(configInitForce)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(daemon.DefaultConfigPath())
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print effective daemon config with secrets redacted",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := daemon.LoadConfig()
		if err != nil {
			return err
		}
		cfg.LayerAPIKey = redactConfigSecret(cfg.LayerAPIKey)
		cfg.S3Key = redactConfigSecret(cfg.S3Key)
		cfg.S3Secret = redactConfigSecret(cfg.S3Secret)
		cfg.S3SessionToken = redactConfigSecret(cfg.S3SessionToken)
		for i := range cfg.Buckets {
			cfg.Buckets[i].Key = redactConfigSecret(cfg.Buckets[i].Key)
			cfg.Buckets[i].Secret = redactConfigSecret(cfg.Buckets[i].Secret)
			cfg.Buckets[i].SessionToken = redactConfigSecret(cfg.Buckets[i].SessionToken)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	},
}

func init() {
	configCmd.AddCommand(newConfigDefaultsCommand())
	configInitCmd.Flags().BoolVar(&configInitForce, "force", false, "Overwrite an existing config file")
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configShowCmd)
}

func redactConfigSecret(value string) string {
	if value == "" {
		return ""
	}
	return "[REDACTED]"
}

func newConfigDefaultsCommand() *cobra.Command {
	var host, path string
	c := &cobra.Command{Use: "defaults", Short: "Show executing node harness/model defaults and sources", Args: cobra.NoArgs}
	c.Flags().StringVar(&host, "host", "", "Query this configured host")
	c.Flags().StringVar(&path, "config", "", "Dispatch configuration")
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		cfg, err := dispatch.Load(path)
		if err != nil && (!os.IsNotExist(err) || path != "" || os.Getenv("HEV_DISPATCH_CONFIG") != "") {
			return err
		}
		var defaults map[dispatch.Role]dispatch.Resolution
		if host != "" {
			h, ok := cfg.Hosts[host]
			if !ok {
				return fmt.Errorf("unknown host %s", host)
			}
			r, err := dispatch.RPC(cmd.Context(), h, dispatch.Request{Action: "config-defaults", Meta: dispatch.Meta{ConfigPath: h.ConfigPath, Order: dispatch.Order{Host: host}}})
			if err != nil {
				return err
			}
			defaults = r.Defaults
			if defaults == nil {
				return fmt.Errorf("host %s does not support role defaults; upgrade target", host)
			}
		} else {
			defaults, err = dispatch.ResolvedDefaults(cfg)
			if err != nil {
				return err
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(defaults)
	}
	return c
}
