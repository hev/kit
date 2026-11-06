package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/index"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/layerenv"
)

func localEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

// Hosted capture uses the same gateway client and service lifecycle as local
// capture, but never starts Docker or puts a gateway in front of the archive.
func runHostedUp(ctx context.Context, out io.Writer, home string, cfg *daemon.Config, key string, noDashboard bool) error {
	endpoint := layerenv.Get("HEVLAYER_ENDPOINT")
	if endpoint == "" && (cfg.LayerStore == "hosted" || cfg.Invite != nil || !localEndpoint(cfg.LayerEndpoint)) {
		endpoint = cfg.LayerEndpoint
	}
	if endpoint == "" {
		endpoint = layerenv.HostedEndpoint
	}
	if key == "" {
		return fmt.Errorf("hosted hev layer requires HEVLAYER_API_KEY")
	}
	namespace := cfg.LayerNamespace
	if namespace == "" {
		namespace = "hev-traces"
	}
	if err := checkInviteKey(ctx, endpoint, key, namespace); err != nil {
		return err
	}
	path := daemon.DefaultConfigPath()
	file, err := loadInitConfig(path)
	if err != nil {
		return err
	}
	target := initLayerConfig{Endpoint: endpoint, APIKey: key, Namespace: namespace, Store: "hosted"}
	migrated := file.Layer.Endpoint != endpoint || file.Layer.Namespace != namespace || file.Layer.APIKey != key
	before := append([]byte(nil), file.source...)
	file.Layer = target
	unlock := func() {}
	if migrated {
		unlock, err = stopArchiveWriter(ctx, home)
		if err != nil {
			return err
		}
	}
	if err := writeInitConfig(path, file); err != nil {
		unlock()
		return err
	}
	after, err := os.ReadFile(path)
	if err != nil {
		unlock()
		return err
	}
	changed := !bytes.Equal(before, after)
	if migrated {
		if err := index.ResetState(); err != nil {
			unlock()
			return err
		}
	}
	unlock()
	configPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Reinstall/restart after a target change so the daemon reads the saved target.
	if hasLaunchd() {
		if err := upDaemon(out, home, configPath, daemon.LocalWrite{Changed: changed}); err != nil {
			return err
		}
	} else if hasSystemd() {
		if err := systemdOwnership(home); err != nil {
			return err
		}
		if err := upSystemd(home, configPath, changed); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(out, "Run `hev d` to capture to hosted hev layer.")
	}
	if _, err := installSkills(home); err != nil {
		return err
	}
	tick(out, "hev layer configured, archiving to namespace %s", namespace)
	if !noDashboard {
		fmt.Fprintln(out, "Dashboard: run `hev open` to sign in to your hosted archive.")
	}
	return nil
}

func stopArchiveWriter(ctx context.Context, home string) (func(), error) {
	if hasSystemd() {
		if err := downSystemd(home); err != nil && !errors.Is(err, errNoSystemdUnit) {
			return nil, err
		}
	}
	return stopInviteWriter(ctx, home)
}
