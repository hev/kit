package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RawBodyFile represents a Claude Code raw API request/response body file
// written by OTEL_LOG_RAW_API_BODIES=file:<dir>.
type RawBodyFile struct {
	Path       string
	Size       int64
	SHA256     string
	LedgerHash string
	Raw        []byte
	Kind       string
	ModTime    time.Time
}

// RolloutFile represents a Codex CLI session rollout JSONL file.
type RolloutFile struct {
	Path       string
	Size       int64
	SHA256     string
	LedgerHash string
	Raw        []byte
	ModTime    time.Time
}

// ScanClaudeRawBodyFiles walks the configured raw-body directory and returns
// request/response JSON files not yet in the ledger.
func ScanClaudeRawBodyFiles(root string, ledger *Ledger, ledgerSalt string) ([]RawBodyFile, error) {
	if root == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var files []RawBodyFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		kind := ""
		switch {
		case strings.HasSuffix(name, ".request.json"):
			kind = "request"
		case strings.HasSuffix(name, ".response.json"):
			kind = "response"
		default:
			return nil
		}

		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		ledgerHash := saltedLedgerHash(hash, ledgerSalt)

		already, err := ledger.IsUploaded(p, ledgerHash)
		if err != nil {
			return nil
		}
		if already {
			return nil
		}

		files = append(files, RawBodyFile{
			Path:       p,
			Size:       fi.Size(),
			SHA256:     hash,
			LedgerHash: ledgerHash,
			Raw:        raw,
			Kind:       kind,
			ModTime:    fi.ModTime(),
		})
		return nil
	})
	return files, err
}

// ScanCodexRolloutFiles walks the configured Codex sessions directory and
// returns rollout JSONL files not yet in the ledger, or whose content changed.
func ScanCodexRolloutFiles(root string, ledger *Ledger, ledgerSalt string) ([]RolloutFile, error) {
	if root == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	var files []RolloutFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "rollout-") || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		ledgerHash := saltedLedgerHash(hash, ledgerSalt)

		already, err := ledger.IsUploaded(p, ledgerHash)
		if err != nil {
			return nil
		}
		if already {
			return nil
		}

		files = append(files, RolloutFile{
			Path:       p,
			Size:       fi.Size(),
			SHA256:     hash,
			LedgerHash: ledgerHash,
			Raw:        raw,
			ModTime:    fi.ModTime(),
		})
		return nil
	})
	return files, err
}

func saltedLedgerHash(hash, salt string) string {
	if salt == "" {
		return hash
	}
	sum := sha256.Sum256([]byte(hash + "\n" + salt))
	return hex.EncodeToString(sum[:])
}
