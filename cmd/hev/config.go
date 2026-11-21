package main

import (
	"encoding/json"
	"fmt"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/license"
	"github.com/hev/kit/internal/redact"
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
			return fmt.Errorf("%s", redact.PublicTokens(err.Error()))
		}
		cfg.LayerAPIKey = redact.ConfigSecret(cfg.LayerAPIKey)
		cfg.S3Key = redact.ConfigSecret(cfg.S3Key)
		cfg.S3Secret = redact.ConfigSecret(cfg.S3Secret)
		cfg.S3SessionToken = redact.ConfigSecret(cfg.S3SessionToken)
		for i := range cfg.Buckets {
			cfg.Buckets[i].Key = redact.ConfigSecret(cfg.Buckets[i].Key)
			cfg.Buckets[i].Secret = redact.ConfigSecret(cfg.Buckets[i].Secret)
			cfg.Buckets[i].SessionToken = redact.ConfigSecret(cfg.Buckets[i].SessionToken)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		lic, _ := license.Configuration()
		lic.Key = redact.ConfigSecret(lic.Key)
		lic.KeyRef = redact.ConfigSecret(lic.KeyRef)
		return enc.Encode(struct {
			*daemon.Config
			License *license.Config `json:"license,omitempty"`
		}{Config: cfg, License: configuredLicense(lic)})
	},
}

func init() {
	configInitCmd.Flags().BoolVar(&configInitForce, "force", false, "Overwrite an existing config file")
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configShowCmd)
}

func configuredLicense(cfg license.Config) *license.Config {
	if cfg == (license.Config{}) {
		return nil
	}
	return &cfg
}
