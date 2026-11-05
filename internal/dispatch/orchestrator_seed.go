package dispatch

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// seedCheckpointSections are the checkpoint parts a seeded round 000 must
// carry, matched case-insensitively at the start of a line after any
// Markdown heading or list marker.
var seedCheckpointSections = []string{"goal status", "open threads", "next action"}

const seedCheckpointLimit = 256 << 10

// ValidateSeedCheckpoint checks that b reads as a checkpoint a round can
// continue from.
func ValidateSeedCheckpoint(b []byte) error {
	if len(b) > seedCheckpointLimit || !utf8.Valid(b) || strings.TrimSpace(string(b)) == "" {
		return fmt.Errorf("seed checkpoint must be non-empty UTF-8 under %d KiB", seedCheckpointLimit>>10)
	}
	found := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.ToLower(strings.TrimLeft(strings.TrimSpace(line), "#*-> \t"))
		for _, section := range seedCheckpointSections {
			if strings.HasPrefix(line, section) {
				found[section] = true
			}
		}
	}
	var missing []string
	for _, section := range seedCheckpointSections {
		if !found[section] {
			missing = append(missing, section)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("seed checkpoint missing sections: %s", strings.Join(missing, ", "))
	}
	return nil
}

// SeedOrchestratorCheckpoint records file as round 000, before the first
// round, with provenance, and makes it the checkpoint round 001 reads.
// Reseeding the same content is a no-op; anything else after round 000 or
// round 001 exists is refused.
func SeedOrchestratorCheckpoint(name, file string) error {
	b, e := os.ReadFile(file)
	if e != nil {
		return e
	}
	if e = ValidateSeedCheckpoint(b); e != nil {
		return e
	}
	if file, e = filepath.Abs(file); e != nil {
		return e
	}
	unlock, e := lockRoot()
	if e != nil {
		return e
	}
	defer unlock()
	st, e := orchestratorState(name)
	if e != nil {
		return e
	}
	dir := OrchestratorDir(name)
	round := filepath.Join(dir, "rounds", "000")
	sum := fmt.Sprintf("%x", sha256.Sum256(b))
	var prior struct {
		SHA256 string `json:"sha256"`
	}
	if readJSON(filepath.Join(round, "provenance.json"), &prior) == nil {
		if prior.SHA256 == sum {
			return nil
		}
		return fmt.Errorf("round 000 already seeded with different content")
	}
	if st.Rounds > 0 {
		return fmt.Errorf("seed checkpoint must precede round 001")
	}
	if e = os.MkdirAll(round, 0700); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(round, "checkpoint.md"), b, 0600); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(round, "trace"), []byte("seed checkpoint from "+file+"\n"), 0600); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "checkpoint.md"), b, 0600); e != nil {
		return e
	}
	// Provenance is written last: it marks the seed complete.
	return WriteJSON(filepath.Join(round, "provenance.json"), map[string]string{"source": file, "sha256": sum, "time": time.Now().UTC().Format(time.RFC3339Nano)})
}
