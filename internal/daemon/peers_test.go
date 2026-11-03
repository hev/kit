package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOptionalPeersFailureStillScans(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	for _, tc := range []struct{ config, want string }{
		{"[peers]\nlisten = 123\n", "line 2"},
		{"[peers]\nlisten = \"" + ln.Addr().String() + "\"\ntoken = \"" + strings.Repeat("x", 32) + "\"\n", ln.Addr().String()},
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(tc.config), 0600)
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		ctx, cancel := context.WithCancel(context.Background())
		close := startOptionalPeers(ctx, path, logger)
		scans := 0
		err := scanLoop(ctx, nil, func() { scans++; cancel() }, time.Hour, logger)
		close()
		if err != nil || scans != 1 || strings.Count(logs.String(), "peers endpoint disabled") != 1 {
			t.Fatal(err, scans, logs.String())
		}
		if !strings.Contains(logs.String(), tc.want) {
			t.Fatal(logs.String())
		}
	}
}
