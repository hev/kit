package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	cmd := &cobra.Command{
		Use: "relay", Short: "Poll once for work from an authenticated order service", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key := os.Getenv("HEV_API_KEY")
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
				(u.Path != "" && u.Path != "/") || host == "" || !strings.HasPrefix(key, "hk_") {
				return errors.New("relay requires an HTTPS origin, host label and HEV_API_KEY")
			}
			u.Path = hostrelay.Path
			return pollRelay(cmd.Context(), u.String(), host, key)
		},
	}
	cmd.Flags().StringVar(&origin, "origin", "", "Order service HTTPS origin")
	cmd.Flags().StringVar(&host, "host", "", "Enrolled execution host label")
	return cmd
}

func pollRelay(ctx context.Context, endpoint, host, key string) error {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
