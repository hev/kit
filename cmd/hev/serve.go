package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/index"
	readserve "github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/trace"
	"github.com/hev/kit/internal/tracefixture"
	"github.com/spf13/cobra"
)

var (
	factoryDir     string
	factoryTeam    string
	factoryRepos   []string
	servePort      int
	serveBind      string
	serveNamespace string
	serveCacheTTL  time.Duration
	serveDemo      bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the local read-side web UI",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if serveDemo {
			return serveDemoTraces(cmd)
		}
		cl, err := client(serveNamespace)
		if err != nil {
			return err
		}
		addr := fmt.Sprintf("%s:%d", serveBind, servePort)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "hev read side: http://%s\n", ln.Addr())
		// Fill picker state only when a picker is opened; Stats needs its own
		// projected snapshot, so startup warming would duplicate that cold read.
		server := readserve.New(cl).WithCacheTTL(serveCacheTTL)
		if factoryDir != "" {
			c := readserve.ResolveFactoryPaths(factoryDir)
			c.Team = factoryTeam
			c.Repos = factoryRepos
			server.WithFactory(c)
		}
		return http.Serve(ln, server.Handler())
	},
}

func init() {
	serveCmd.Flags().StringVar(&factoryDir, "factory-dir", "", "Directory containing factory sessions, prices, subscriptions and outcome cache")
	serveCmd.Flags().StringVar(&factoryTeam, "factory-team", "", "Configured Linear team scope for cached outcomes")
	serveCmd.Flags().StringSliceVar(&factoryRepos, "factory-repos", nil, "Configured repository scope for cached outcomes")
	serveCmd.Flags().IntVar(&servePort, "port", 8787, "Local TCP port (0 chooses a free port)")
	serveCmd.Flags().StringVar(&serveBind, "bind", "127.0.0.1", "Address to listen on; a tailscale address lets one laptop reach a headless box")
	serveCmd.Flags().StringVar(&serveNamespace, "namespace", "", "Layer namespace (default from config)")
	serveCmd.Flags().BoolVar(&serveDemo, "demo-traces", false, "Serve a built-in ask-trace fixture from $HOME (written there if absent) instead of Layer; for local checks only")
	serveCmd.Flags().DurationVar(&serveCacheTTL, "cache-ttl", readserve.DefaultCacheTTL, "How long filter pickers count from the cached archive before it is revalidated against Layer in the background (0 disables)")
}

// serveDemoTraces writes the trace fixture under $HOME when it is not there
// yet, derives its session rows in memory exactly as the indexer would, and
// serves the dashboard over them. Point HOME at a scratch directory.
func serveDemoTraces(cmd *cobra.Command) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	info := tracefixture.Info{Transcripts: filepath.Join(home, ".claude", "projects")}
	if _, err := os.Stat(filepath.Join(dispatch.Root(), tracefixture.OrderNav)); os.IsNotExist(err) {
		if info, err = tracefixture.Write(home, time.Now()); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "wrote trace fixture under %s (trace %s)\n", home, info.Trace)
	}
	sessions, blocks, err := index.ReadSide(&trace.ClaudeSource{Root: info.Transcripts}, "fixture")
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serveBind, servePort))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "hev read side (demo traces, %d sessions): http://%s/#asks\n", len(sessions), ln.Addr())
	server := readserve.New(&readserve.MemoryReader{Sessions: sessions, Blocks: blocks}).WithCacheTTL(0)
	return http.Serve(ln, server.Handler())
}
