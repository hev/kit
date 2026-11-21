package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	boardserver "github.com/hev/kit/internal/board/server"
	boardsettings "github.com/hev/kit/internal/board/settings"
	boardstore "github.com/hev/kit/internal/board/store"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/index"
	readserve "github.com/hev/kit/internal/serve"
	"github.com/hev/kit/internal/storagemode"
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
		if k := kitSignedIn(); k != nil && serveNamespace == "" && factoryDir == "" {
			ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serveBind, servePort))
			if err != nil {
				return err
			}
			return serveKitDashboard(cmd.OutOrStdout(), ln, kitClient(k), k.TeamSlug)
		}
		cl, images, err := tracesWithImages(serveNamespace)
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
		server := readserve.New(cl).WithCacheTTL(serveCacheTTL).WithBoard(serveBoard(cmd.OutOrStdout()))
		if images != nil && images.Store != nil {
			// Images whose bytes this host captured are shown inline.
			server.WithImages(images.Store)
		}
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
	serveCmd.Flags().BoolVar(&serveDemo, "demo-traces", false, "Serve a built-in trace fixture from $HOME (written there if absent) instead of Layer; for local checks only")
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
	fmt.Fprintf(cmd.OutOrStdout(), "hev read side (demo traces, %d sessions): http://%s/#traces\n", len(sessions), ln.Addr())
	server := readserve.New(&readserve.MemoryReader{Sessions: sessions, Blocks: blocks}).WithCacheTTL(0).WithBoard(serveBoard(cmd.OutOrStdout()))
	return http.Serve(ln, server.Handler())
}

// serveBoard opens the board the Board tab reads, the same way hev board
// decides: a server board.url names; otherwise this host's files with no
// Layer key, or the Layer namespace with one; a machine set up for Layer
// whose shell has no key reads its local board server. A board that cannot
// be opened keeps the tab and says so; the reason is printed here, for the
// operator, and never sent to the browser.
func serveBoard(out io.Writer) *readserve.Board {
	unavailable := func(err error) *readserve.Board {
		fmt.Fprintf(out, "board: unavailable: %v\n", err)
		return readserve.UnavailableBoard()
	}
	if err := boardsettings.Load(); err != nil {
		return unavailable(err)
	}
	remote := func(what string) *readserve.Board {
		b, err := readserve.RemoteBoard(boardsettings.Current.URL, readserve.DefaultBoardTimeout)
		if err != nil {
			return unavailable(err)
		}
		fmt.Fprintf(out, "board: %s\n", what)
		return b
	}
	if boardsettings.Current.URLSet {
		return remote("the board server board.url names")
	}
	st, err := boardstore.Open()
	if storagemode.IsNoKey(err) {
		return remote("the board server on this host (this shell has no Layer key)")
	}
	if err != nil {
		return unavailable(err)
	}
	srv, err := boardserver.New(st)
	if err != nil {
		return unavailable(err)
	}
	fmt.Fprintf(out, "board: %s mode\n", st.Info().Mode)
	return readserve.LocalBoard(srv.Handler(), st.Info().Mode)
}
