package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/hev/kit/internal/update"
	"github.com/hev/kit/internal/version"
	"github.com/spf13/cobra"
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Channel   string `json:"channel"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}

// currentVersionInfo describes this binary. The commit comes from the edge
// version's suffix or the build's VCS stamp; channel is "dev" for a binary
// built without a published version.
func currentVersionInfo() versionInfo {
	v := version.String()
	info := versionInfo{Version: v, Channel: "dev", OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version()}
	if n, err := update.NormalizeVersion(v); err == nil {
		info.Channel = update.VersionChannel(n)
		if i := strings.LastIndex(n, "-edge."); i >= 0 {
			info.Commit = n[i+len("-edge."):]
		}
	}
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, s := range b.Settings {
			if s.Key == "vcs.revision" && (info.Commit == "" || strings.HasPrefix(s.Value, info.Commit)) {
				info.Commit = s.Value
			}
		}
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	return info
}

func newVersionCommand() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print the hev version",
		Long: `Print the hev version.

Plain output is the version string alone, which update checks and release
scripts compare exactly. --json adds commit, channel, os/arch and Go version.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !asJSON {
				fmt.Fprintln(cmd.OutOrStdout(), version.String())
				return nil
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(currentVersionInfo())
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print version, commit, channel, os/arch and Go version as JSON")
	return c
}
