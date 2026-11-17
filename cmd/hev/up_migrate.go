package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/filearchive"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/local"
	"github.com/hev/kit/internal/trace"
)

// The retired local lane kept the archive in Layer CE on Postgres, in Docker.
// `hev up` moves such a config to files in one of two ways:
//
//  1. export: when docker is here and the lane's containers start, every row
//     (chunks, blocks, sessions, evals, instruction versions) is paged out of
//     the old gateway into the file archive, so sessions whose transcripts
//     are gone survive; the containers kit started are stopped again;
//  2. re-index: otherwise the index state is reset so hevd re-reads every
//     transcript still on disk, and up says how many sessions are not on disk
//     any more and stay only in the Docker volume.
//
// Neither removes a container or the volume.

// migrationGatewayWait bounds how long up waits for the old gateway to
// answer after starting its containers.
var migrationGatewayWait = 90 * time.Second

func retiredEndpoint(cfg *daemon.Config) string {
	if e := strings.TrimSpace(cfg.LayerEndpoint); e != "" && localEndpoint(e) {
		return strings.TrimRight(e, "/")
	}
	port := cfg.Local.Port
	if port == 0 {
		port = daemon.DefaultLocalPort
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func migrateRetired(ctx context.Context, out io.Writer, cfg *daemon.Config) error {
	project := cfg.Local.Project
	if project == "" {
		project = daemon.DefaultLocalProject
	}
	namespace := cfg.LayerNamespace
	if namespace == "" {
		namespace = "hev-traces"
	}
	fmt.Fprintf(out, "  • %s names the retired local Docker lane (Layer CE on Postgres): moving it to files\n", cfg.ConfigPath)
	dst, err := filearchive.Open(filearchive.Root(cfg.ArchiveDir), namespace)
	if err != nil {
		return err
	}
	n, err := exportRetired(ctx, out, cfg, project, namespace, dst)
	if err == nil {
		tick(out, "exported %s from the old Postgres archive into %s", n, dst.Dir())
		tick(out, "the Docker volume is left as it is; remove it yourself when you no longer need it (docker volume ls --filter label=com.docker.compose.project=%s)", project)
		return nil
	}
	fmt.Fprintf(out, "  • could not export from the old archive (%v): re-indexing the transcripts still on disk instead\n", err)
	onDisk, gone := transcriptsOnDisk(index.LoadState())
	if err := index.ResetState(); err != nil {
		return err
	}
	tick(out, "%d sessions will be re-indexed from transcripts still on disk", onDisk)
	if gone > 0 {
		fmt.Fprintf(out, "  ! %d sessions could not be recovered: their transcripts are gone from disk; they remain only in the old Docker volume (project %s)\n", gone, project)
	} else {
		tick(out, "every previously indexed session still has its transcript on disk")
	}
	return nil
}

// exportCounts words what an export moved.
type exportCounts map[string]int

func (c exportCounts) String() string {
	var parts []string
	for _, k := range []string{"sessions", "chunks", "blocks", "evals", "instruction versions"} {
		parts = append(parts, fmt.Sprintf("%d %s", c[k], k))
	}
	return strings.Join(parts, ", ")
}

// exportRetired starts the lane's containers if they exist and are stopped,
// waits for its gateway and copies every row into dst. Containers it started
// are stopped again.
func exportRetired(ctx context.Context, out io.Writer, cfg *daemon.Config, project, namespace string, dst *filearchive.Store) (exportCounts, error) {
	if err := local.Docker(ctx); err != nil {
		return nil, err
	}
	cs, err := local.Containers(ctx, project)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return nil, fmt.Errorf("no containers of Compose project %s", project)
	}
	started, err := local.Start(ctx, cs)
	if err != nil {
		return nil, err
	}
	if len(started) > 0 {
		tick(out, "started %d stopped container(s) of %s to read the old archive", len(started), project)
		defer func() {
			if err := local.Stop(context.Background(), started); err != nil {
				fmt.Fprintf(out, "  ! could not stop them again: %v\n", err)
			} else {
				tick(out, "stopped them again")
			}
		}()
	}
	key := strings.TrimSpace(cfg.LayerAPIKey)
	if key == "" {
		key = layerenv.PlaceholderKey
	}
	src := layer.New(retiredEndpoint(cfg), key, namespace, "")
	if err := waitFor(ctx, migrationGatewayWait, func() error { _, err := src.Health(); return err }); err != nil {
		return nil, fmt.Errorf("the old gateway on %s did not answer: %w", src.Endpoint, err)
	}
	return exportArchive(src, dst)
}

// exportArchive copies every namespace of src into dst, rows unchanged.
func exportArchive(src *layer.Client, dst *filearchive.Store) (exportCounts, error) {
	n := exportCounts{}
	for _, suffix := range layer.ExportSuffixes {
		err := src.ExportRows(suffix, 500, func(raw []json.RawMessage) error {
			switch suffix {
			case "":
				rows, err := decodeAll[layer.Row](raw)
				if err != nil {
					return err
				}
				n["chunks"] += len(rows)
				_, err = dst.Write(rows)
				return err
			case "-blocks":
				rows, err := decodeAll[trace.BlockRow](raw)
				if err != nil {
					return err
				}
				n["blocks"] += len(rows)
				_, err = dst.WriteBlocks(rows)
				return err
			case "-sessions":
				rows, err := decodeAll[trace.SessionRow](raw)
				if err != nil {
					return err
				}
				n["sessions"] += len(rows)
				_, err = dst.WriteSessions(rows)
				return err
			case "-evals":
				rows, err := decodeAll[layer.EvalRow](raw)
				if err != nil {
					return err
				}
				evals := make([]trace.Eval, 0, len(rows))
				for _, r := range rows {
					e, err := r.Eval()
					if err != nil {
						return err
					}
					evals = append(evals, *e)
				}
				n["evals"] += len(evals)
				_, err = dst.WriteEvals(evals)
				return err
			case "-instructions":
				rows, err := decodeAll[layer.InstructionVersion](raw)
				if err != nil {
					return err
				}
				n["instruction versions"] += len(rows)
				return dst.WriteInstructionVersions(rows)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("export %s%s: %w", src.Namespace, suffix, err)
		}
	}
	return n, nil
}

func decodeAll[T any](raw []json.RawMessage) ([]T, error) {
	out := make([]T, len(raw))
	for i, r := range raw {
		if err := json.Unmarshal(r, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// transcriptsOnDisk counts the transcripts the index state says were
// indexed (one per session; an OpenCode unit is a session in a shared file),
// by whether they are still on disk.
func transcriptsOnDisk(st *index.State) (onDisk, gone int) {
	seen := map[string]bool{}
	for key := range st.Units {
		// <archive key>:<path>[#<session>]
		_, unit, ok := strings.Cut(key, ":")
		if path, _, _ := strings.Cut(unit, "#"); !ok || seen[unit] || !filepath.IsAbs(path) {
			continue // dispatch trace spans are not transcripts
		}
		seen[unit] = true
	}
	units := make([]string, 0, len(seen))
	for u := range seen {
		units = append(units, u)
	}
	sort.Strings(units)
	for _, u := range units {
		path, _, _ := strings.Cut(u, "#")
		if _, err := os.Stat(path); err == nil {
			onDisk++
		} else {
			gone++
		}
	}
	return onDisk, gone
}
