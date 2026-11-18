package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hev/kit/internal/collect"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/spf13/cobra"
)

var (
	collectListen          string
	collectNamespaces      string
	collectTracesNamespace string
	collectBoardsNamespace string
	collectEnvelopesDir    string
)

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Receive a host app's OTLP traces and serve its trace search and memory board API",
	Long: `Run app capture for a host app (an agent on the Vercel AI SDK, for example).

hev collect takes OTLP/HTTP trace exports on /v1/traces, scrubs every span with
kit's redaction policy, writes each span as a trace envelope, collapses each
turn's spans into one turn document, and indexes it for hybrid search in the
configured Layer target. On the same listener it serves the app API:

  GET  /healthz
  POST /v1/app/traces/search
  GET  /v1/app/traces/conversations/{chatId}?namespace=&owner=&tenant=
  POST /v1/app/boards/{board}/posts
  POST /v1/app/boards/{board}/search

Every route but /healthz needs Authorization: Bearer $HEV_COLLECT_SERVICE_KEY.
The key is per host app: it lets the app assert owner and author, so it never
goes to a browser or a model.

Namespaces keep environments apart. The app declares its namespace
(<app>-<env>, e.g. story-prod) as the hev.namespace resource attribute, and
--namespaces (HEV_COLLECT_NAMESPACES, required) lists the namespaces this
collector's key may write and read. Spans without an allowed namespace are
refused; API calls name a namespace and read only it. A namespace's turns
live in the Layer namespace <--traces-namespace>-<namespace>, for example
hev-app-traces-story-prod. It runs no transcript tailing, dispatch, peers or
dashboard. See docs/app-traces.md.`,
	Args: cobra.NoArgs,
	RunE: runCollect,
}

func init() {
	collectCmd.Flags().StringVar(&collectListen, "listen", envOrDefault("HEV_COLLECT_LISTEN", "127.0.0.1:4318"), "Address to listen on (OTLP/HTTP and the app API)")
	collectCmd.Flags().StringVar(&collectNamespaces, "namespaces", os.Getenv("HEV_COLLECT_NAMESPACES"), "Comma-separated app namespaces the service key may write and read, e.g. story-prod (required)")
	collectCmd.Flags().StringVar(&collectTracesNamespace, "traces-namespace", envOrDefault("HEV_COLLECT_TRACES_NAMESPACE", collect.DefaultTracesPrefix), "Prefix of each app namespace's Layer namespace for turn documents (<prefix>-<namespace>)")
	collectCmd.Flags().StringVar(&collectBoardsNamespace, "boards-namespace", envOrDefault("HEV_COLLECT_BOARDS_NAMESPACE", "hev-app-boards"), "Layer namespace for app boards")
	collectCmd.Flags().StringVar(&collectEnvelopesDir, "envelopes", os.Getenv("HEV_COLLECT_ENVELOPES_DIR"), "Directory for trace envelopes (default ~/.hev/collect/envelopes)")
	rootCmd.AddCommand(collectCmd)
}

func envOrDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func runCollect(cmd *cobra.Command, _ []string) error {
	key := os.Getenv("HEV_COLLECT_SERVICE_KEY")
	if key == "" {
		return collect.ErrNoKey
	}
	namespaces, err := collect.ParseNamespaces(collectNamespaces)
	if err != nil {
		return err
	}
	prefix := strings.TrimSpace(collectTracesNamespace)
	if prefix == "" {
		return errors.New("--traces-namespace must not be empty")
	}
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))

	scrubber, err := redact.Load()
	if err != nil {
		return fmt.Errorf("redaction: %w", err)
	}
	indexes := map[string]collect.TurnIndex{}
	layerNamespaces := make([]string, 0, len(namespaces))
	var endpoint string
	for _, ns := range namespaces {
		lns := collect.LayerNamespace(prefix, ns)
		c, err := client(lns)
		if err != nil {
			return err
		}
		endpoint = c.Endpoint
		indexes[ns] = &collect.LayerTurns{Client: c, Namespace: ns}
		layerNamespaces = append(layerNamespaces, lns)
	}
	boardsClient, err := client(collectBoardsNamespace)
	if err != nil {
		return err
	}
	dir := collectEnvelopesDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".hev", "collect", "envelopes")
	}
	srv, err := collect.New(collect.Config{
		Key:      key,
		Indexes:  indexes,
		Boards:   collect.NewStoreBoards(boardsClient),
		Sink:     &collect.FileSink{Dir: dir},
		Scrubber: scrubber,
		Host:     layer.Hostname(),
		Logger:   logger,
	})
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", collectListen)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() { srv.Run(ctx); close(done) }()

	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	redaction := "basic"
	if scrubber == nil {
		redaction = "off (capture.redact = false)"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "hev collect: listening on http://%s (layer %s, namespaces %s, turns %s, boards %s, envelopes %s, redaction %s)\n",
		ln.Addr(), endpoint, strings.Join(namespaces, ","), strings.Join(layerNamespaces, ","), collectBoardsNamespace, dir, redaction)

	select {
	case err := <-errc:
		stop()
		<-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = hs.Shutdown(shutdown)
	<-done
	return err
}
