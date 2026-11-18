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
	"syscall"
	"time"

	"github.com/hev/kit/internal/collect"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/spf13/cobra"
)

var (
	collectListen          string
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
  GET  /v1/app/traces/conversations/{chatId}?owner=&tenant=
  POST /v1/app/boards/{board}/posts
  POST /v1/app/boards/{board}/search

Every route but /healthz needs Authorization: Bearer $HEV_COLLECT_SERVICE_KEY.
The key is per host app: it lets the app assert owner and author, so it never
goes to a browser or a model. It runs no transcript tailing, dispatch, peers or
dashboard. See docs/app-traces.md.`,
	Args: cobra.NoArgs,
	RunE: runCollect,
}

func init() {
	collectCmd.Flags().StringVar(&collectListen, "listen", envOrDefault("HEV_COLLECT_LISTEN", "127.0.0.1:4318"), "Address to listen on (OTLP/HTTP and the app API)")
	collectCmd.Flags().StringVar(&collectTracesNamespace, "traces-namespace", envOrDefault("HEV_COLLECT_TRACES_NAMESPACE", "hev-app-traces"), "Layer namespace for turn documents")
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
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))

	scrubber, err := redact.Load()
	if err != nil {
		return fmt.Errorf("redaction: %w", err)
	}
	turnsClient, err := client(collectTracesNamespace)
	if err != nil {
		return err
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
		Index:    &collect.LayerTurns{Client: turnsClient},
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
	fmt.Fprintf(cmd.OutOrStdout(), "hev collect: listening on http://%s (layer %s, turns %s, boards %s, envelopes %s, redaction %s)\n",
		ln.Addr(), turnsClient.Endpoint, collectTracesNamespace, collectBoardsNamespace, dir, redaction)

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
