package main

import (
	"strings"

	"github.com/spf13/cobra"
)

const tracesLong = `List, inspect and search the sessions hevd and hev index captured.

  hev traces list [--since 3d]          sessions, newest first
  hev traces show <id>                  one session's transcript (id prefix)
  hev traces search <query> [--also …]  search every captured session
  hev traces images <id> [--out DIR]    the images a session saw; --out saves them

Images in transcripts (screenshots, pasted mocks, computer-use frames) are
captured with the session: the bytes stay on the capturing host under the
archive directory (<archive>/<namespace>/blobs), the rows carry metadata, and
the text carries a marker like [image image/png 390x700 7.5 KB sha256:3f2a9c1b04de].
"hev traces images <id> --out DIR" saves them so another agent can look.
Turn byte capture off with [capture] images = false or HEVD_CAPTURE_IMAGES=0.`

func addDirectSurface() {
	addRemovedVerbs()
	rootCmd.AddCommand(newNodesCommand())
	rootCmd.AddCommand(newAdmissionCommand())
	rootCmd.AddCommand(newPeersCommand(false))
	traces := &cobra.Command{Use: "traces", Short: "List, inspect and search captured traces", Long: tracesLong, Args: cobra.MaximumNArgs(1), RunE: runLs}
	traces.Flags().AddFlagSet(lsCmd.Flags())
	// Reparent the existing trace handlers under the sole 0.3 interface.
	for _, pair := range []struct {
		name   string
		source *cobra.Command
	}{{"list", lsCmd}, {"show", traceCmd}, {"search", queryCmd}} {
		src := pair.source
		use := strings.Fields(src.Use)
		use[0] = pair.name
		src.Use = strings.Join(use, " ")
		src.Aliases = nil
		if pair.name == "list" {
			src.Aliases = []string{"ls"}
		}
		traces.AddCommand(src)
	}
	traces.AddCommand(traceImagesCmd)
	rootCmd.AddCommand(traces)
}
