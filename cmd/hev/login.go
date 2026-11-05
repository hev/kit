package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/index"
	"github.com/hev/kit/internal/platformops"
	"github.com/spf13/cobra"
)

func init() {
	login := &cobra.Command{Use: "login", Short: "Sign this machine into a team archive", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, _ := cmd.Flags().GetString("service")
		return runLogin(cmd.Context(), cmd.OutOrStdout(), service)
	}}
	login.Flags().String("service", "", "Team sign-in service URL")
	_ = login.MarkFlagRequired("service")
	rootCmd.AddCommand(login, &cobra.Command{Use: "logout", Short: "Revoke this machine's team key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return runLogout(cmd.Context(), cmd.OutOrStdout()) }})
}
func validTeamURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))
}
func loginRandom() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func teamPost(ctx context.Context, service, path, key string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", service+path, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("team service unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("team service returned HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out)
	}
	return nil
}

// The callback accepts only the fresh state and never exposes a code in output.
func loginCallback(state string, codes chan<- string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != "GET" || r.URL.Path != "/callback" || r.URL.Query().Get("state") != state || r.URL.Query().Get("code") == "" {
			http.Error(w, "invalid callback", 400)
			return
		}
		select {
		case codes <- r.URL.Query().Get("code"):
			fmt.Fprintln(w, "Signed in. Return to your terminal.")
		default:
			http.Error(w, "callback already received", 409)
		}
	})
}
func runLogin(ctx context.Context, out io.Writer, service string) error {
	if err := platformops.RequireTraceWrites(); err != nil {
		return err
	}
	if !validTeamURL(service) {
		return fmt.Errorf("service must use HTTPS or loopback HTTP")
	}
	service = strings.TrimRight(service, "/")
	for _, name := range []string{"LAYER_ENDPOINT", "LAYER_API_KEY", "LAYER_NAMESPACE", "LAYER_STORE"} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("unset %s before login", name)
		}
	}
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	if doc["invite"] != nil || doc["team"] != nil {
		return fmt.Errorf("leave or logout before selecting another archive")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	state, verifier := loginRandom(), loginRandom()
	sum := sha256.Sum256([]byte(verifier))
	codes := make(chan string, 1)
	server := &http.Server{Handler: loginCallback(state, codes), ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	u, _ := url.Parse(service + "/cli/login")
	q := u.Query()
	q.Set("state", state)
	q.Set("challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("port", fmt.Sprint(listener.Addr().(*net.TCPAddr).Port))
	u.RawQuery = q.Encode()
	fmt.Fprintln(out, "Open to sign in:", u.String())
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", u.String()).Run()
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var code string
	select {
	case code = <-codes:
	case <-ctx.Done():
		return fmt.Errorf("sign-in timed out or canceled")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	var result struct {
		Endpoint, Namespace, Key string
		KeyID                    string `json:"key_id"`
		Author                   struct{ ID, Name, Email string }
	}
	if err = teamPost(ctx, service, "/cli/exchange", "", map[string]string{"code": code, "verifier": verifier, "hostname": hostname}, &result); err != nil {
		return err
	}
	if !validTeamURL(result.Endpoint) || result.Namespace == "" || strings.ContainsAny(result.Namespace, "/ ?#") || result.Key == "" || result.KeyID == "" || result.Author.ID == "" {
		return fmt.Errorf("invalid team response")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unlock, err := stopInviteWriter(ctx, home)
	if err != nil {
		return err
	}
	defer unlock()
	if result.Author.Name == "" {
		result.Author.Name = result.Author.ID
	}
	team := daemon.TeamConfig{Service: service, KeyID: result.KeyID}
	team.Author.ID, team.Author.Name, team.Author.Email = result.Author.ID, result.Author.Name, result.Author.Email
	if err = daemon.WriteTeamConfig(doc, result.Endpoint, result.Namespace, result.Key, team); err != nil {
		return err
	}
	if err = index.ResetState(); err != nil {
		return err
	}
	fmt.Fprintln(out, "Signed in as", result.Author.Name, "· namespace", result.Namespace, "· run hev d to capture sessions")
	return nil
}
func runLogout(ctx context.Context, out io.Writer) error {
	if err := platformops.RequireTraceWrites(); err != nil {
		return err
	}
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Team == nil {
		return fmt.Errorf("no team login configured")
	}
	if !validTeamURL(cfg.Team.Service) {
		return fmt.Errorf("invalid team service URL")
	}
	if err = teamPost(ctx, cfg.Team.Service, "/cli/logout", teamFileKey(doc), nil, nil); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unlock, err := stopInviteWriter(ctx, home)
	if err != nil {
		return err
	}
	defer unlock()
	if err = daemon.ClearTeamConfig(doc); err != nil {
		return err
	}
	if err = index.ResetState(); err != nil {
		return err
	}
	fmt.Fprintln(out, "This machine is signed out; previous archive settings restored.")
	return nil
}

func teamFileKey(doc map[string]any) string {
	target, _ := doc["layer"].(map[string]any)
	key, _ := target["api_key"].(string)
	return key
}
