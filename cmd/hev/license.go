package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hev/kit/internal/license"
	"github.com/spf13/cobra"
)

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Inspect Kit's offline Pro license",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		s := license.Resolve(cmd.Context())
		machine, _ := cmd.Flags().GetBool("json")
		if machine {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(s)
		}
		writeKitLicense(cmd.OutOrStdout(), s)
		return nil
	},
}

func init() {
	licenseCmd.Flags().Bool("json", false, "Print license status as JSON")
	rootCmd.AddCommand(licenseCmd)
}

func kitEdition(s license.Status) string {
	if s.Entitled() {
		return "hev kit pro"
	}
	return "hev kit OSS"
}

func licenseLine(s license.Status) string {
	if s.Reason != "" {
		return fmt.Sprintf("%s (%s)", s.State, s.Reason)
	}
	return string(s.State)
}

func writeKitLicense(out io.Writer, s license.Status) {
	fmt.Fprintf(out, "edition     %s\nlicense     %s\n", kitEdition(s), licenseLine(s))
	if s.Expires != nil {
		fmt.Fprintf(out, "expires     %s\n", s.Expires.Format(time.RFC3339))
	}
	if len(s.Features) != 0 {
		fmt.Fprintf(out, "features    %q\n", strings.Join(s.Features, ", "))
	}
	if warning := s.Warning(); warning != "" {
		fmt.Fprintf(out, "  ! %s\n", warning)
	}
}
