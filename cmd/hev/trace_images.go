package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hev/kit/internal/blob"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/platformops"
	tracepkg "github.com/hev/kit/internal/trace"
)

var (
	traceImagesOut  string
	traceImagesJSON bool
)

var traceImagesCmd = &cobra.Command{
	Use:   "images <id>",
	Short: "List a session's images, or save them with --out DIR",
	Long: `List the images a session's transcript carried, in transcript order:
block, role, kind, media type, dimensions, size and sha256. With --out DIR,
save each one whose bytes are on this host and print its path, so an agent
can look at what another session saw.

Images are captured by hev index and hevd: the bytes go to this host's blob
store under the archive directory (<archive>/<namespace>/blobs) in both
storage modes, and the archive rows carry only the metadata. An image
captured on another host is listed with that host's name and is not
fetched. An image that was over the size cap, not an accepted type, or seen
while capture was off ([capture] images = false, HEVD_CAPTURE_IMAGES=0) is
listed as skipped, with the reason.

--json prints the same fields, plus each saved path.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl, images, err := tracesWithImages(traceNamespace)
		if err != nil {
			return err
		}
		var store *blob.Store
		if images != nil {
			store = images.Store
		}
		return runTraceImages(cmd.OutOrStdout(), cl, store, layer.Hostname(), args[0], traceImagesOut, traceImagesJSON)
	},
}

func init() {
	traceImagesCmd.Flags().StringVar(&traceNamespace, "namespace", "", "Layer namespace (default $HEVLAYER_NAMESPACE or hev-traces)")
	traceImagesCmd.Flags().StringVar(&traceImagesOut, "out", "", "Save the images that are on this host into DIR and print their paths")
	traceImagesCmd.Flags().BoolVar(&traceImagesJSON, "json", false, "Print the list as JSON")
}

// sessionImage is one image in a session, in transcript order.
type sessionImage struct {
	N         int    `json:"n"`
	Seq       int64  `json:"seq"`
	TurnUUID  string `json:"turn_uuid"`
	Role      string `json:"role"`
	BlockType string `json:"block_type"`
	ToolName  string `json:"tool_name,omitempty"`
	Index     int    `json:"index"`
	SHA256    string `json:"sha256,omitempty"`
	Type      string `json:"type,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Size      int64  `json:"size"`
	Skipped   string `json:"skipped,omitempty"`
	Host      string `json:"host,omitempty"`
	OnHost    bool   `json:"on_host"`
	Path      string `json:"path,omitempty"`
}

// resolveSession finds the one session a prefix names.
func resolveSession(cl traceStore, prefix string) (tracepkg.SessionRow, error) {
	rows, err := cl.SessionRows(prefix)
	if err != nil {
		return tracepkg.SessionRow{}, err
	}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.SessionID] = true
	}
	if len(ids) == 0 {
		return tracepkg.SessionRow{}, fmt.Errorf("no trace matching prefix %q", prefix)
	}
	if len(ids) > 1 {
		return tracepkg.SessionRow{}, fmt.Errorf("trace prefix %q is ambiguous (%d sessions); use more of the id", prefix, len(ids))
	}
	var id string
	for k := range ids {
		id = k
	}
	session := tracepkg.SessionRow{SessionID: id}
	if found, err := cl.ListSessionRows(-1, []any{"session_id", "Eq", id}); err == nil && len(found) > 0 {
		session = found[0]
	}
	return session, nil
}

// listSessionImages reads the session's block rows and returns its images
// in transcript order, each marked by whether its bytes are on this host.
func listSessionImages(cl traceStore, store *blob.Store, session tracepkg.SessionRow) ([]sessionImage, error) {
	blocks, err := cl.ListBlockRows(session.SessionID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Seq < blocks[j].Seq })
	var out []sessionImage
	for _, b := range blocks {
		for _, im := range b.Images {
			si := sessionImage{N: len(out) + 1, Seq: b.Seq, TurnUUID: b.TurnUUID, Role: b.Role, BlockType: b.BlockType,
				ToolName: b.ToolName, Index: im.Index, SHA256: im.SHA256, Type: im.Type, Width: im.Width, Height: im.Height,
				Size: im.Size, Skipped: im.Skipped, Host: session.Host}
			si.OnHost = im.Stored() && store != nil && blob.ValidHash(im.SHA256) && fileExists(store.Path(im.SHA256))
			out = append(out, si)
		}
	}
	return out, nil
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

func runTraceImages(w io.Writer, cl traceStore, store *blob.Store, thisHost, prefix, outDir string, asJSON bool) error {
	session, err := resolveSession(cl, prefix)
	if err != nil {
		return err
	}
	images, err := listSessionImages(cl, store, session)
	if err != nil {
		return err
	}
	var missing []string
	if outDir != "" {
		if err := platformops.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
		for i := range images {
			im := &images[i]
			if im.Skipped != "" {
				continue
			}
			if !im.OnHost {
				missing = append(missing, fmt.Sprintf("image %d (%s) is not on this host%s", im.N, short(im.SHA256), capturedOn(im.Host, thisHost)))
				continue
			}
			b, m, err := store.Get(im.SHA256)
			if err != nil {
				missing = append(missing, fmt.Sprintf("image %d (%s): %v", im.N, short(im.SHA256), err))
				continue
			}
			p := filepath.Join(outDir, fmt.Sprintf("%03d-%s%s", im.N, short(m.SHA256), blob.Ext(m.Type)))
			if err := platformops.WriteFileAtomic(p, b, 0o644); err != nil {
				return err
			}
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
			im.Path = p
		}
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if images == nil {
			images = []sessionImage{}
		}
		if err := enc.Encode(images); err != nil {
			return err
		}
	} else {
		writeImageList(w, session, images, thisHost, outDir != "")
	}
	for _, m := range missing {
		fmt.Fprintln(os.Stderr, "not saved: "+m)
	}
	if len(missing) > 0 {
		return errors.New("some images could not be saved; see above")
	}
	return nil
}

func writeImageList(w io.Writer, session tracepkg.SessionRow, images []sessionImage, thisHost string, saved bool) {
	if len(images) == 0 {
		fmt.Fprintf(w, "no images in session %s\n", session.SessionID)
		return
	}
	fmt.Fprintf(w, "%d images in session %s%s\n\n", len(images), session.SessionID, capturedOn(session.Host, thisHost))
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tBLOCK\tROLE\tKIND\tTYPE\tDIMENSIONS\tSIZE\tSHA256\tWHERE")
	for _, im := range images {
		kind := im.BlockType
		if im.ToolName != "" {
			kind += " (" + im.ToolName + ")"
		}
		dims := ""
		if im.Width > 0 {
			dims = fmt.Sprintf("%dx%d", im.Width, im.Height)
		}
		where := ""
		switch {
		case im.Skipped != "":
			where = "skipped: " + im.Skipped
		case im.Path != "":
			where = im.Path
		case im.OnHost:
			where = "on this host"
		default:
			where = "not on this host" + capturedOn(im.Host, thisHost)
		}
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", im.N, im.Seq, im.Role, kind, im.Type, dims, blob.Human(im.Size), short(im.SHA256), where)
	}
	tw.Flush()
	if !saved {
		fmt.Fprintf(w, "\nsave them: hev traces images %s --out DIR\n", session.SessionID)
	}
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// capturedOn names the capturing host, unless it is this one.
func capturedOn(host, thisHost string) string {
	if host == "" || host == thisHost {
		return ""
	}
	return " (captured on " + host + ")"
}
