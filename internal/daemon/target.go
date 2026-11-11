package daemon

import (
	"fmt"
	"strings"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/trace"

	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/storagemode"
)

// RetiredStore is the store kind the retired local Docker lane (Layer CE on
// Postgres with pgvector) wrote into
// [layer].store.
const RetiredStore = "pgvector"

// Target is where this host's traces are kept: Layer when a key is
// configured, files when none is.
type Target struct {
	Mode  storagemode.Mode
	Layer *layer.Client
	Files *filearchive.Store
	// Retired is set when the config still names the retired local Docker
	// store; the caller prints RetiredMessage.
	Retired bool
	// Images is where this host keeps the image bytes its transcripts
	// carried: <archive root>/<namespace>/blobs in both modes, because Layer
	// has no object store. Off when [capture] images is false.
	Images *trace.ImageCapture
}

// ImageCapture is the blob store for a namespace's images on this host.
// It creates nothing and never fails: a namespace that cannot be a
// directory name (Layer accepts more than the file archive does) leaves the
// capture with no store, and each image it meets is skipped with a marker
// saying why, so commands that store no image behave as they did before
// images were captured.
func ImageCapture(cfg *Config, namespace string) (*trace.ImageCapture, error) {
	off := !cfg.CaptureImages
	dir, err := filearchive.BlobsDir(filearchive.Root(cfg.ArchiveDir), namespace)
	if err != nil {
		return &trace.ImageCapture{Off: off, Unavailable: fmt.Sprintf("namespace %q cannot hold image files on this host", namespace)}, nil
	}
	return &trace.ImageCapture{Store: blob.New(dir, 0), Off: off}, nil
}

// Archive is the indexer's view of the target.
func (t Target) Archive() index.Archive {
	if t.Files != nil {
		return t.Files
	}
	return t.Layer
}

// OpenTarget decides the trace storage mode with storagemode.Resolve and
// opens it. A key in [layer].api_key counts (that is where hosted `hev up`
// saves it); the placeholder "local" does not. A key that is configured but
// cannot be read is an error, never a switch to files. namespace overrides
// the configured one when set.
func OpenTarget(cfg *Config, namespace string) (Target, error) {
	if namespace == "" {
		namespace = cfg.LayerNamespace
	}
	if namespace == "" {
		namespace = "hev-traces"
	}
	retired := strings.EqualFold(strings.TrimSpace(cfg.LayerStore), RetiredStore)
	images, err := ImageCapture(cfg, namespace)
	if err != nil {
		return Target{}, err
	}
	res, err := storagemode.Resolve(storagemode.Options{ConfigKey: true, ConfigPath: cfg.ConfigPath, Peek: true})
	if err != nil {
		return Target{}, err
	}
	if res.Mode == storagemode.Filesystem {
		files, err := filearchive.Open(filearchive.Root(cfg.ArchiveDir), namespace)
		if err != nil {
			return Target{}, err
		}
		return Target{Mode: storagemode.Filesystem, Files: files, Retired: retired, Images: images}, nil
	}
	key := cfg.LayerAPIKey
	if strings.TrimSpace(key) == layerenv.PlaceholderKey {
		key = ""
	}
	if key == "" {
		if key, err = LayerKey(); err != nil {
			return Target{}, fmt.Errorf("a Layer API key is configured but could not be read: %w", err)
		}
	}
	endpoint := cfg.LayerEndpoint
	if endpoint == "" {
		endpoint = layer.DefaultEndpoint
	}
	store := cfg.LayerStore
	if retired {
		store = ""
	}
	client, err := layer.New(endpoint, key, namespace, layerenv.Get("HEVLAYER_EMBED_MODEL")).WithStore(layer.StoreKind(store))
	if err != nil {
		return Target{}, err
	}
	return Target{Mode: storagemode.Layer, Layer: client, Retired: retired, Images: images}, nil
}

// RetiredMessage is what kit says to a host whose config still names the
// retired local Postgres store. It never stops, starts or deletes anything.
func RetiredMessage(cfg *Config) string {
	project := cfg.Local.Project
	if project == "" {
		project = DefaultLocalProject
	}
	return fmt.Sprintf(`The local Docker archive lane (Layer CE on Postgres) is retired.
With no Layer API key, kit keeps traces in files under %s and needs no Docker.
Nothing was moved or deleted.

  Old sessions: hevd re-reads every transcript still on disk into the file
  archive on its next scans (or run `+"`hev index`"+` now). Sessions whose
  transcripts are gone stay only in the old Docker volume. To keep traces in
  Layer instead, set HEVLAYER_API_KEY and bring them across with
  `+"`hev backfill`"+`.

  Stop and remove the old containers yourself when you no longer need them:
    docker compose -p %s down
  The archive volume is left alone. Remove it only if you are sure:
    docker volume ls --filter label=com.docker.compose.project=%s
    docker volume rm <volume>

  `+"`hev up`"+` moves this config to files: it drops store = %q and
  api_key = %q from [layer] in %s.`,
		filearchive.Root(cfg.ArchiveDir), project, project, RetiredStore, layerenv.PlaceholderKey, cfg.ConfigPath)
}
