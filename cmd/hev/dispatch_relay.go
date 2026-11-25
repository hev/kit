package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/pkg/kitapi"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hev/kit/pkg/hostrelay"
	"github.com/hev/kit/pkg/orders/v1"
	"github.com/hev/kit/pkg/peers/v1"
	"github.com/spf13/cobra"
)

func dispatchRelayCommand() *cobra.Command {
	var origin, host string
	var syncPeers bool
	cmd := &cobra.Command{
		Use: "relay", Short: "Poll once for work from an authenticated order service", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key := os.Getenv("HEV_API_KEY")
			base := origin
			selected, err := currentWorkspace()
			if err != nil {
				return err
			}
			ctx := dispatch.WorkspaceContext(cmd.Context(), selected)
			if cfg := kitSignedIn(); cfg != nil && !selected.IsolatedStorage() {
				if base == "" {
					base = cfg.API
					if key == "" {
						key = cfg.Key
					}
				} else if key == "" && strings.TrimRight(base, "/") == strings.TrimRight(cfg.API, "/") {
					key = cfg.Key
				}
			}
			u, err := url.Parse(base)
			if err != nil || !kitapi.ValidBaseURL(base) || u.Scheme != "https" ||
				(u.Path != "" && u.Path != "/") || !dispatch.ValidLabel(host) || !selected.AllowsHost(host) || !kitapi.ValidKey(key) {
				return errors.New("relay requires an HTTPS origin, a host label and a kit key")
			}
			u.Path = hostrelay.Path
			if err := pollRelay(ctx, u.String(), host, key); err != nil {
				return err
			}
			if syncPeers {
				return relayPeerPresence(ctx, base, host, key)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&origin, "origin", "", "Order service HTTPS origin")
	cmd.Flags().StringVar(&host, "host", "", "Enrolled execution host label")
	cmd.Flags().BoolVar(&syncPeers, "sync-peers", false, "Publish local inbox peers and their receipts after polling")
	return cmd
}

// relayTransport is overridden in tests to verify HTTPS relay requests without
// live network I/O. Nil uses http.DefaultTransport.
var relayTransport http.RoundTripper

func pollRelay(ctx context.Context, endpoint, host, key string) error {
	client := &http.Client{
		Timeout:       15 * time.Second,
		Transport:     relayTransport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	call := func(method string, body []byte) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("invalid relay request")
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, errors.New("host relay unavailable")
		}
		return resp, nil
	}
	resp, err := call(http.MethodGet, nil)
	if err != nil {
		return err
	}
	var job hostrelay.Job
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&job)
	resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode != http.StatusOK || decodeErr != nil || job.ID == "" || (job.Order == nil) == (job.Peer == nil) {
		return fmt.Errorf("invalid relay response (%d)", resp.StatusCode)
	}
	result := hostrelay.Result{ID: job.ID}
	if job.Order != nil {
		if job.Order.Host != host {
			return errors.New("relay job addressed to another host")
		}
		raw, _ := json.Marshal(job.Order)
		reply := orders.HandleRPC(ctx, string(raw))
		result.Order = &reply
	} else {
		h, _, _ := strings.Cut(job.Peer.ID, ":")
		if h != host || job.Peer.Action != "deliver" {
			return errors.New("invalid peer relay operation")
		}
		backend := peers.RPCBackend{"relay": {Host: host}}
		reply, err := backend.Execute(ctx, "relay", *job.Peer)
		if err != nil {
			reply.Status = http.StatusServiceUnavailable
		}
		result.Peer = &reply
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return errors.New("invalid relay result")
	}
	resp, err = call(http.MethodPost, raw)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("relay result refused (%d)", resp.StatusCode)
	}
	return nil
}

func relayPeerPresence(ctx context.Context, base, host, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	backend := peers.RPCBackend{"relay": {Host: host}}
	local, err := backend.Execute(ctx, "relay", peers.Request{Action: "list"})
	if err != nil || local.Status != 200 {
		return errors.New("local peers unavailable")
	}
	var rows []map[string]any
	if json.Unmarshal(local.Body, &rows) != nil {
		return errors.New("invalid local peers response")
	}
	client := kitapi.New(base, key)
	client.Retries = 0
	for _, row := range rows {
		id, _ := row["id"].(string)
		incarnation, _ := row["recipient_incarnation"].(string)
		if id == "" || !strings.HasPrefix(id, host+":") || incarnation == "" || row["inbox"] != true || row["role"] == "offline" {
			continue
		}
		path := "/v1/peers/" + url.PathEscape(id)
		for _, field := range []string{"id", "address", "name", "host", "last_seen", "stale", "inbox_status"} {
			delete(row, field)
		}
		if err := client.Do(ctx, http.MethodPut, path, nil, row, nil); err != nil {
			return errors.New("peer heartbeat refused")
		}
		msgs, err := backend.Execute(ctx, "relay", peers.Request{Action: "messages", ID: id})
		if err != nil || msgs.Status != 200 {
			return errors.New("local inbox unavailable")
		}
		if err := relayReceipts(ctx, client, path, incarnation, msgs.Body); err != nil {
			return err
		}
	}
	return nil
}

func relayReceipts(ctx context.Context, client *kitapi.Client, path, incarnation string, raw json.RawMessage) error {
	var body struct {
		Data []struct {
			ID             string `json:"id"`
			Current        bool   `json:"current"`
			ConsumedAt     string `json:"consumed_at"`
			AcknowledgedAt string `json:"acknowledged_at"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return errors.New("invalid local inbox response")
	}
	for _, kind := range []string{"consumed", "acknowledged"} {
		ids := []string{}
		for _, message := range body.Data {
			if message.Current && message.ID != "" && message.ConsumedAt != "" && (kind == "consumed" || message.AcknowledgedAt != "") {
				ids = append(ids, message.ID)
			}
		}
		for len(ids) > 0 {
			n := min(64, len(ids))
			doc := map[string]any{"kind": kind, "ids": ids[:n], "recipient_incarnation": incarnation}
			if err := client.Do(ctx, http.MethodPost, path+"/receipts", nil, doc, nil); err != nil {
				return errors.New("peer receipts refused")
			}
			ids = ids[n:]
		}
	}
	return nil
}
