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
	for _, config := range []string{"[peers]\nlisten = 123\n", "[peers]\nlisten = \"" + ln.Addr().String() + "\"\ntoken = \"" + strings.Repeat("x", 32) + "\"\n"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(config), 0600)
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
		want := ln.Addr().String()
		if strings.Contains(config, "123") {
			want = "line 2"
		}
		if !strings.Contains(logs.String(), want) {
			t.Fatal(logs.String())
		}
	}
}
