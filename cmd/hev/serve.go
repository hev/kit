package main

import (
	"fmt"
	"net"
	"net/http"
	"time"

	readserve "github.com/hev/kit/internal/serve"
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
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the local read-side web UI",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
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
		server := readserve.New(cl).WithCacheTTL(serveCacheTTL).Warm()
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
	serveCmd.Flags().DurationVar(&serveCacheTTL, "cache-ttl", readserve.DefaultCacheTTL, "How long filter pickers count from the cached archive before it is revalidated against Layer in the background (0 disables)")
}
