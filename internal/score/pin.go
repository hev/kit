package score

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/platformops"
)

const (
	// MaxPinnedFileBytes caps individual scorer command/data files hashed into a pin set (512 MiB).
	MaxPinnedFileBytes = 512 << 20
	// MaxPinStoreBytes caps the host pin store file size (16 MiB).
	MaxPinStoreBytes = 16 << 20
	// MaxPinEntries bounds the number of pinned scorers in one host pin store.
	MaxPinEntries = 4096
)

var (
	// ErrUnpinned indicates the scorer has no pin set recorded in the host pin store.
	ErrUnpinned = errors.New("scorer is not pinned")
	// ErrStalePin indicates one or more files in the scorer's pin set no longer match.
	ErrStalePin = errors.New("scorer pin set is stale")
)

// PinnedFile records one host file's canonical path and SHA-256 hex digest.
type PinnedFile struct {
	Path   string `json:"path" toml:"path"`
	SHA256 string `json:"sha256" toml:"sha256"`
}

// PinSet records the complete host-side SHA-256 pin set for a scorer.
type PinSet struct {
	Scorer   string       `json:"scorer" toml:"scorer"`
	SpecPath string       `json:"spec_path" toml:"spec_path"`
	Pin      string       `json:"pin" toml:"pin"`
	Spec     PinnedFile   `json:"spec" toml:"spec"`
	Command  PinnedFile   `json:"command" toml:"command"`
	Data     []PinnedFile `json:"data,omitempty" toml:"data,omitempty"`
	PinnedAt string       `json:"pinned_at" toml:"pinned_at"`
}

type pinStoreFile struct {
	Version int      `toml:"version"`
	Pins    []PinSet `toml:"pins"`
}

// PinVerificationError describes why a scorer's pin check was refused.
type PinVerificationError struct {
	Scorer     string   `json:"scorer,omitempty"`
	Status     string   `json:"status"` // "unpinned" or "stale"
	Mismatches []string `json:"mismatches,omitempty"`
}

func (e *PinVerificationError) Error() string {
	label := e.Scorer
	if label == "" {
		label = "scorer"
	}
	if e.Status == "unpinned" {
		return fmt.Sprintf("%s is unpinned (run `hev score pin` on the host first)", label)
	}
	if len(e.Mismatches) == 0 {
		return fmt.Sprintf("%s pin set is stale", label)
	}
	return fmt.Sprintf("%s pin set is stale: %s", label, strings.Join(e.Mismatches, "; "))
}

func (e *PinVerificationError) Unwrap() error {
	if e.Status == "unpinned" {
		return ErrUnpinned
	}
	return ErrStalePin
}

// DefaultPinStorePath resolves the host-side scorer pin store location.
// Order of precedence:
//  1. HEV_SCORE_PINS environment variable
//  2. <DefaultScorerDir>/pins.toml (~/.config/hev/scorers/pins.toml)
func DefaultPinStorePath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("HEV_SCORE_PINS")); p != "" {
		return p, nil
	}
	dir, err := DefaultScorerDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pins.toml"), nil
}

// DefaultScorerDir returns the host scorer directory (~/.config/hev/scorers, or HEV_SCORE_DIR).
func DefaultScorerDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("HEV_SCORE_DIR")); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "hev", "scorers"), nil
}

// ComputePinSet loads and validates specPath and hashes the spec file, its
// command executable/script, and every declared data file.
func ComputePinSet(specPath string) (Spec, PinSet, error) {
	spec, err := LoadSpec(specPath)
	if err != nil {
		return Spec{}, PinSet{}, err
	}
	resolved, err := ResolvePaths(specPath, spec)
	if err != nil {
		return Spec{}, PinSet{}, err
	}
	specSum, err := hashRegularFile(resolved.Spec)
	if err != nil {
		return Spec{}, PinSet{}, fmt.Errorf("spec file: %w", err)
	}
	cmdSum, err := hashRegularFile(resolved.Command)
	if err != nil {
		return Spec{}, PinSet{}, fmt.Errorf("command file: %w", err)
	}
	dataPins := make([]PinnedFile, len(resolved.Data))
	for i, dp := range resolved.Data {
		sum, err := hashRegularFile(dp)
		if err != nil {
			return Spec{}, PinSet{}, fmt.Errorf("data[%d]: %w", i, err)
		}
		dataPins[i] = PinnedFile{Path: dp, SHA256: sum}
	}
	ps := PinSet{
		Scorer:   spec.Name,
		SpecPath: resolved.Spec,
		Spec:     PinnedFile{Path: resolved.Spec, SHA256: specSum},
		Command:  PinnedFile{Path: resolved.Command, SHA256: cmdSum},
		Data:     dataPins,
		PinnedAt: time.Now().UTC().Format(time.RFC3339),
	}
	ps.Pin = compositeDigest(ps)
	return spec, ps, nil
}

// Pin computes the SHA-256 pin set for specPath and atomically persists it into storePath.
func Pin(storePath, specPath string) (PinSet, error) {
	if strings.TrimSpace(storePath) == "" {
		var err error
		storePath, err = DefaultPinStorePath()
		if err != nil {
			return PinSet{}, err
		}
	}
	_, ps, err := ComputePinSet(specPath)
	if err != nil {
		return PinSet{}, err
	}
	if err := platformops.MkdirAll(filepath.Dir(storePath), 0700); err != nil {
		return PinSet{}, err
	}
	unlock, err := platformops.LockPath(storePath+".lock", false)
	if err != nil {
		return PinSet{}, fmt.Errorf("lock pin store: %w", err)
	}
	defer unlock()

	store, err := readPinStore(storePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return PinSet{}, err
	}
	replaced := false
	for i := range store.Pins {
		if samePinKey(store.Pins[i].SpecPath, ps.SpecPath) {
			store.Pins[i] = ps
			replaced = true
			break
		}
	}
	if !replaced {
		if len(store.Pins) >= MaxPinEntries {
			return PinSet{}, fmt.Errorf("pin store entry limit (%d) reached", MaxPinEntries)
		}
		store.Pins = append(store.Pins, ps)
	}
	sort.Slice(store.Pins, func(i, j int) bool {
		return store.Pins[i].SpecPath < store.Pins[j].SpecPath
	})
	store.Version = 1

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(store); err != nil {
		return PinSet{}, err
	}
	if err := platformops.WriteFileAtomic(storePath, buf.Bytes(), 0600); err != nil {
		return PinSet{}, err
	}
	return ps, nil
}

// Verify checks that specPath has a recorded pin set in storePath and that the
// spec file, command executable/script, and every data file match their pinned
// SHA-256 digests.
func Verify(storePath, specPath string) (Spec, PinSet, error) {
	if strings.TrimSpace(storePath) == "" {
		var err error
		storePath, err = DefaultPinStorePath()
		if err != nil {
			return Spec{}, PinSet{}, err
		}
	}
	if strings.TrimSpace(specPath) == "" {
		return Spec{}, PinSet{}, fmt.Errorf("scorer file path is required")
	}
	canonSpec, err := canonicalLeafPath(specPath)
	if err != nil {
		return Spec{}, PinSet{}, err
	}
	fallbackLabel := defaultNameFromPath(canonSpec)

	store, err := readPinStore(storePath)
	if errors.Is(err, os.ErrNotExist) {
		return Spec{}, PinSet{}, &PinVerificationError{Scorer: fallbackLabel, Status: "unpinned"}
	}
	if err != nil {
		return Spec{}, PinSet{}, err
	}

	var stored *PinSet
	for i := range store.Pins {
		if samePinKey(store.Pins[i].SpecPath, canonSpec) {
			stored = &store.Pins[i]
			break
		}
	}
	if stored == nil {
		return Spec{}, PinSet{}, &PinVerificationError{Scorer: fallbackLabel, Status: "unpinned"}
	}
	scorerName := stored.Scorer
	if scorerName == "" {
		scorerName = fallbackLabel
	}

	var mismatches []string
	if stored.Pin == "" || stored.Pin != compositeDigest(*stored) {
		mismatches = append(mismatches, "pin manifest digest mismatch")
	}

	specSum, err := hashRegularFile(canonSpec)
	if err != nil {
		mismatches = append(mismatches, fmt.Sprintf("spec (%s): %v", filepath.Base(canonSpec), err))
	} else if specSum != stored.Spec.SHA256 {
		mismatches = append(mismatches, fmt.Sprintf("spec (%s): digest mismatch (got %s, pinned %s)", filepath.Base(canonSpec), shortHex(specSum), shortHex(stored.Spec.SHA256)))
	}

	var spec Spec
	var resolved ResolvedPaths
	var specLoaded bool
	if s, lerr := LoadSpec(canonSpec); lerr == nil {
		spec = s
		if r, rerr := ResolvePaths(canonSpec, s); rerr == nil {
			resolved = r
			specLoaded = true
		} else {
			mismatches = append(mismatches, fmt.Sprintf("spec paths: %v", rerr))
		}
	} else if err == nil {
		mismatches = append(mismatches, fmt.Sprintf("spec invalid: %v", lerr))
	}

	if specLoaded && !samePinKey(resolved.Command, stored.Command.Path) {
		mismatches = append(mismatches, fmt.Sprintf("command path changed to %s", filepath.Base(resolved.Command)))
	}
	cmdSum, err := hashRegularFile(stored.Command.Path)
	if err != nil {
		mismatches = append(mismatches, fmt.Sprintf("command (%s): %v", filepath.Base(stored.Command.Path), err))
	} else if cmdSum != stored.Command.SHA256 {
		mismatches = append(mismatches, fmt.Sprintf("command (%s): digest mismatch (got %s, pinned %s)", filepath.Base(stored.Command.Path), shortHex(cmdSum), shortHex(stored.Command.SHA256)))
	}

	if specLoaded && len(resolved.Data) != len(stored.Data) {
		mismatches = append(mismatches, fmt.Sprintf("data file count changed (got %d, pinned %d)", len(resolved.Data), len(stored.Data)))
	}
	for i, dp := range stored.Data {
		if specLoaded && i < len(resolved.Data) && !samePinKey(resolved.Data[i], dp.Path) {
			mismatches = append(mismatches, fmt.Sprintf("data[%d] path changed to %s", i, filepath.Base(resolved.Data[i])))
		}
		sum, err := hashRegularFile(dp.Path)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("data[%d] (%s): %v", i, filepath.Base(dp.Path), err))
		} else if sum != dp.SHA256 {
			mismatches = append(mismatches, fmt.Sprintf("data[%d] (%s): digest mismatch (got %s, pinned %s)", i, filepath.Base(dp.Path), shortHex(sum), shortHex(dp.SHA256)))
		}
	}

	if len(mismatches) > 0 {
		return Spec{}, *stored, &PinVerificationError{
			Scorer:     scorerName,
			Status:     "stale",
			Mismatches: mismatches,
		}
	}
	return spec, *stored, nil
}

func readPinStore(path string) (pinStoreFile, error) {
	var out pinStoreFile
	info, err := os.Lstat(path)
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() {
		return out, fmt.Errorf("pin store must be a regular file")
	}
	if info.Size() > MaxPinStoreBytes {
		return out, fmt.Errorf("pin store exceeds %d bytes", MaxPinStoreBytes)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	md, err := toml.Decode(string(b), &out)
	if err != nil {
		return out, fmt.Errorf("decode pin store: %w", err)
	}
	if len(md.Undecoded()) > 0 {
		return out, fmt.Errorf("pin store contains unknown fields")
	}
	if out.Version != 0 && out.Version != 1 {
		return out, fmt.Errorf("unsupported pin store version %d", out.Version)
	}
	return out, nil
}

func hashRegularFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("missing file")
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file")
	}
	if info.Size() > MaxPinnedFileBytes {
		return "", fmt.Errorf("file exceeds %d bytes", MaxPinnedFileBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, MaxPinnedFileBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func compositeDigest(ps PinSet) string {
	h := sha256.New()
	fmt.Fprintf(h, "pinset:v1\nscorer:%s\nspec:%s\ncommand:%s\n", ps.Scorer, ps.Spec.SHA256, ps.Command.SHA256)
	for i, d := range ps.Data {
		fmt.Fprintf(h, "data[%d]:%s\n", i, d.SHA256)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func samePinKey(a, b string) bool {
	ca, errA := canonicalLeafPath(a)
	cb, errB := canonicalLeafPath(b)
	if errA == nil && errB == nil {
		return ca == cb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func shortHex(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
