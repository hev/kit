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
	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/storagemode"
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
turn's spans into one turn document, and stores it for search. Storage
follows kit's rule (docs/design/storage-modes.md): with a Layer API key, turns
and app boards go to hev layer and search is hybrid; with no key, they go to
the file archive on this host (HEV_ARCHIVE_DIR, [archive] dir, default
~/.hev/traces, or a legacy ~/.hev/archive) and search is grep: every word of the query, no meaning.
A key that is configured but cannot be read is an error, never files. On the
same listener it serves the app API:

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
live in the Layer (or archive) namespace <--traces-namespace>-<namespace>, for
example hev-app-traces-story-prod. It runs no transcript tailing, dispatch, peers or
dashboard. See docs/app-traces.md.`,
	Args: cobra.NoArgs,
	RunE: runCollect,
}

func init() {
	collectCmd.Flags().StringVar(&collectListen, "listen", envOrDefault("HEV_COLLECT_LISTEN", "127.0.0.1:4318"), "Address to listen on (OTLP/HTTP and the app API)")
	collectCmd.Flags().StringVar(&collectNamespaces, "namespaces", os.Getenv("HEV_COLLECT_NAMESPACES"), "Comma-separated app namespaces the service key may write and read, e.g. story-prod (required)")
	collectCmd.Flags().StringVar(&collectTracesNamespace, "traces-namespace", envOrDefault("HEV_COLLECT_TRACES_NAMESPACE", collect.DefaultTracesPrefix), "Prefix of each app namespace's Layer namespace for turn documents (<prefix>-<namespace>)")
	collectCmd.Flags().StringVar(&collectBoardsNamespace, "boards-namespace", envOrDefault("HEV_COLLECT_BOARDS_NAMESPACE", "hev-app-boards"), "Layer namespace for app boards (with no key, the directory <archive root>/<this>)")
	collectCmd.Flags().StringVar(&collectEnvelopesDir, "envelopes", os.Getenv("HEV_COLLECT_ENVELOPES_DIR"), "Directory for trace envelopes (default ~/.hev/collect/envelopes with a Layer key, <archive root>/hev-app-envelopes without)")
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
	st, err := openCollectStores(namespaces, prefix, collectBoardsNamespace, collectEnvelopesDir)
	if err != nil {
		return err
	}
	srv, err := collect.New(collect.Config{
		Key:      key,
		Indexes:  st.indexes,
		Boards:   st.boards,
		Sink:     &collect.FileSink{Dir: st.envelopes},
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
	fmt.Fprintf(cmd.OutOrStdout(), "hev collect: listening on http://%s (%s, namespaces %s, turns %s, boards %s, envelopes %s, redaction %s)\n",
		ln.Addr(), st.where, strings.Join(namespaces, ","), strings.Join(st.turns, ","), st.boardsAt, st.envelopes, redaction)

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

// collectStores is where hev collect keeps turns, app boards and envelopes.
type collectStores struct {
	mode      storagemode.Mode
	indexes   map[string]collect.TurnIndex
	boards    collect.Boards
	envelopes string
	// where, turns and boardsAt describe the stores for the startup line.
	// They never carry a key.
	where    string
	turns    []string
	boardsAt string
}

// collectEnvelopesName is the envelope directory under the archive root
// when there is no Layer key.
const collectEnvelopesName = "hev-app-envelopes"

// openCollectStores decides the storage mode the way hevd does
// (daemon.ResolveMode: storagemode.Resolve, plus files only when nothing on
// the machine says Layer) and opens the stores. With a key: Layer, as
// before. With none: the file archive under its root (HEV_ARCHIVE_DIR,
// [archive] dir, ~/.hev/traces), one archive namespace per app namespace
// named as the Layer namespace would be, app boards in <root>/<boards
// namespace>, envelopes in <root>/hev-app-envelopes unless --envelopes says
// otherwise. A key that is configured but cannot be read is an error, never
// a fall back to files.
func openCollectStores(namespaces []string, prefix, boardsNS, envelopes string) (*collectStores, error) {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return nil, err
	}
	res, err := daemon.ResolveMode(cfg)
	if err != nil {
		return nil, err
	}
	st := &collectStores{mode: res.Mode, indexes: map[string]collect.TurnIndex{}, envelopes: envelopes}
	if res.Mode == storagemode.Filesystem {
		root, err := filepath.Abs(filearchive.Root(cfg.ArchiveDir))
		if err != nil {
			return nil, err
		}
		for _, ns := range namespaces {
			fns := collect.LayerNamespace(prefix, ns)
			files, err := filearchive.Open(root, fns)
			if err != nil {
				return nil, err
			}
			st.indexes[ns] = &collect.FileTurns{Store: files, Namespace: ns}
			st.turns = append(st.turns, fns)
		}
		boardsDir, err := filearchive.NamespaceDir(root, boardsNS)
		if err != nil {
			return nil, err
		}
		if st.boards, err = collect.NewFileBoards(boardsDir); err != nil {
			return nil, err
		}
		st.boardsAt = boardsDir
		if st.envelopes == "" {
			st.envelopes = filepath.Join(root, collectEnvelopesName)
		}
		st.where = "files " + root + ", grep search (words, not meaning)"
		return st, nil
	}
	var endpoint string
	for _, ns := range namespaces {
		lns := collect.LayerNamespace(prefix, ns)
		c, err := client(lns)
		if err != nil {
			return nil, err
		}
		endpoint = c.Endpoint
		st.indexes[ns] = &collect.LayerTurns{Client: c, Namespace: ns}
		st.turns = append(st.turns, lns)
	}
	boardsClient, err := client(boardsNS)
	if err != nil {
		return nil, err
	}
	st.boards = collect.NewStoreBoards(boardsClient)
	st.boardsAt = boardsNS
	if st.envelopes == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		st.envelopes = filepath.Join(home, ".hev", "collect", "envelopes")
	}
	st.where = "layer " + endpoint
	return st, nil
}
