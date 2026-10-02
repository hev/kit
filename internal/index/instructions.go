package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hev/kit/internal/layer"
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/internal/trace"
)

// RunInstructions uses archive state rather than the local signature cache so
// restarts, target changes and failed writes cannot silently lose history.
func RunInstructions(src trace.InstructionSource, cl *layer.Client) (*Report, error) {
	unlock, err := redact.LockArchive()
	if err != nil {
		return nil, err
	}
	defer unlock()
	scrubber, err := redact.Load()
	if err != nil {
		return nil, err
	}
	files, err := src.Files()
	if err != nil {
		return nil, err
	}
	rep := &Report{UnitsSeen: len(files), Redactions: redact.Counts{}}
	for _, f := range files {
		if err := indexInstruction(f, cl, scrubber, rep); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", f.Path, err))
		}
	}
	return rep, nil
}
func indexInstruction(f trace.InstructionFile, cl *layer.Client, scrubber *redact.Scrubber, rep *Report) error {
	host := layer.Hostname()
	versions, err := cl.InstructionVersions(f.Path, host)
	if err != nil {
		return err
	}
	var current *layer.InstructionVersion
	for i := range versions {
		if versions[i].ValidTo == "" {
			if current != nil {
				return fmt.Errorf("multiple current instruction versions")
			}
			current = &versions[i]
		}
	}
	if current != nil && current.ContentHash == f.Hash {
		rep.UnitsSkipped++
		return nil
	}
	previous := ""
	if current != nil {
		previous = current.ID
	}
	sum := sha256.Sum256([]byte(host + "\x00" + f.Path + "\x00" + f.Hash + "\x00" + previous))
	id := hex.EncodeToString(sum[:])
	clean, counts := scrubber.Text(f.Text)
	rep.Redactions.Add(counts)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	v := layer.InstructionVersion{ID: id, Path: f.Path, Project: f.Project, Host: host, Mtime: f.Mtime, ContentHash: f.Hash, ValidFrom: now, ChunkIDs: []string{}}
	t := trace.Turn{SessionID: "instruction-" + id, TurnUUID: id, Harness: "instructions", SourcePath: f.Path, Workdir: f.Project, TS: f.Mtime, Role: "instruction", Blocks: []trace.Block{{Type: "text", Text: clean}}}
	var rows []layer.Row
	for _, chunk := range trace.ChunksMax(t, cl.ChunkRunes()) {
		row := layer.RowOf(chunk)
		row.Path = f.Path
		row.Project = f.Project
		row.VersionID = id
		rows = append(rows, row)
		v.ChunkIDs = append(v.ChunkIDs, row.ID)
	}
	for start := 0; start < len(rows); start += 200 {
		result, err := cl.Write(rows[start:min(start+200, len(rows))])
		if err != nil {
			return err
		}
		rep.RowsUpserted += result.RowsUpserted
		rep.EmbeddingTokens += result.EmbeddingTokens
	}
	// Close the prior version and publish its replacement in one metadata write.
	batch := []layer.InstructionVersion{v}
	if current != nil {
		current.ValidTo = now
		batch = append(batch, *current)
	}
	if err := cl.WriteInstructionVersions(batch); err != nil {
		return err
	}
	rep.Chunks += len(rows)
	rep.UnitsIndexed++
	return nil
}
