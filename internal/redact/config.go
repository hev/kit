package redact

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"
)

// ConfigPath follows the same HEV_CONFIG override as the daemon.
func ConfigPath() (string, error) {
	if p := os.Getenv("HEV_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hev", "config.toml"), nil
}

// Load reads capture.redact (default true) and securely persists a random
// capture.redact_salt. A nil scrubber means explicit opt-out. Callers must stop
// ingestion on errors, never silently fall back to raw text.
func Load() (*Scrubber, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFile(path)
}

// LoadFile is also used by archive migration to share the ingestion identity.
func LoadFile(path string) (*Scrubber, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	// Lock first creation across CLI and daemon processes, not merely goroutines.
	fd, err := unix.Open(path+".redact.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("redaction config lock: %w", err)
	}
	defer unix.Close(fd)
	if err = unix.Flock(fd, unix.LOCK_EX); err != nil {
		return nil, err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("redaction config must be a regular file")
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var cfg struct {
		Capture struct {
			Redact *bool  `toml:"redact"`
			Salt   string `toml:"redact_salt"`
		} `toml:"capture"`
	}
	if _, err = toml.Decode(string(b), &cfg); err != nil {
		return nil, fmt.Errorf("redaction config: %w", err)
	}
	if cfg.Capture.Redact != nil && !*cfg.Capture.Redact {
		return nil, nil
	}
	if cfg.Capture.Salt != "" {
		salt, err := hex.DecodeString(cfg.Capture.Salt)
		if err != nil || len(salt) != 32 {
			return nil, fmt.Errorf("capture.redact_salt must be 32 bytes encoded as hex")
		}
		if err = os.Chmod(path, 0600); err != nil {
			return nil, err
		}
		return New(salt)
	}
	// An explicitly empty field is an invalid identity, not permission to rotate.
	var generic map[string]any
	if _, err = toml.Decode(string(b), &generic); err != nil {
		return nil, err
	}
	if capture, ok := generic["capture"].(map[string]any); ok {
		if _, ok := capture["redact_salt"]; ok {
			return nil, fmt.Errorf("capture.redact_salt is empty")
		}
	}
	salt := make([]byte, 32)
	if _, err = rand.Read(salt); err != nil {
		return nil, err
	}
	line := "redact_salt = \"" + hex.EncodeToString(salt) + "\"\n"
	text := string(b)
	// Insert at the beginning of the capture table, preserving all existing
	// comments, unknown settings and credentials exactly as the operator wrote them.
	header := regexp.MustCompile(`(?m)^\s*\[capture\][ \t]*(?:#[^\n]*)?\r?$`).FindStringIndex(text)
	if header == nil {
		if strings.Contains(text, "capture.") {
			return nil, fmt.Errorf("use a [capture] table before persisting redaction salt")
		}
		text += "\n[capture]\n" + line
	} else {
		pos := header[1]
		text = text[:pos] + "\n" + line + text[pos:]
	}
	// Validate before replacing: unusual TOML table syntax must fail closed.
	if _, err = toml.Decode(text, &cfg); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".redact-config-*")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.WriteString(text); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(name, path); err != nil {
		return nil, err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return nil, err
	}
	return New(salt)
}
