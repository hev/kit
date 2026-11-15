package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/hev/kit/internal/daemon"
	"github.com/hev/kit/internal/kitcapture"
	"github.com/hev/kit/internal/layerenv"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/spf13/cobra"
)

// kitServiceEnv names the sign-in service when neither --service nor the
// config's [kit] service does.
const kitServiceEnv = "HEV_KIT_SERVICE"

const defaultKitService = "https://kit.hevmind.com"

// openBrowser is replaced in tests by a fake browser.
var openBrowser = func(u string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", u).Run()
	}
}

// loginTimeout bounds the wait for the browser callback.
var loginTimeout = 3 * time.Minute

func init() {
	login := &cobra.Command{Use: "login", Short: "Verify and save a hosted API key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, _ := cmd.Flags().GetString("service")
		invite, _ := cmd.Flags().GetString("code")
		importHistory, _ := cmd.Flags().GetBool("import")
		browser, _ := cmd.Flags().GetBool("browser")
		key, _ := cmd.Flags().GetString("key")
		if importHistory && key == "" && daemon.ResolveAPIKey("") == "" && !browser && kitSignedIn() != nil {
			// Already signed in: import (or resume importing) only.
			return runKitImport(cmd.Context(), cmd.OutOrStdout())
		}
		var err error
		if browser {
			err = runLogin(cmd.Context(), cmd.OutOrStdout(), service, invite)
		} else {
			if key == "" {
				key = daemon.ResolveAPIKey("")
			}
			if key == "" {
				raw, e := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 4096))
				if e != nil {
					return e
				}
				key = strings.TrimSpace(string(raw))
			}
			err = runKeyLogin(cmd.Context(), cmd.OutOrStdout(), service, key)
		}
		if err != nil {
			return err
		}
		if importHistory {
			return runKitImport(cmd.Context(), cmd.OutOrStdout())
		}
		offerKitImport(cmd.OutOrStdout())
		return nil
	}}
	login.Flags().Bool("browser", false, "Use the legacy browser sign-in flow")
	login.Flags().String("key", "", "API key (prefer HEV_API_KEY or stdin to avoid shell history)")
	login.Flags().String("service", "", "kit sign-in service URL (default "+defaultKitService+"; overridden by "+kitServiceEnv+" or [kit] service)")
	originalHelp := login.HelpFunc()
	login.SetHelpFunc(func(cmd *cobra.Command, args []string) { originalHelp(cmd, args); showEarnedCode(cmd) })
	login.Flags().Bool("import", false, "Copy this machine's trace archive and board to the kit API (resumable; run again after signing in to resume)")
	login.Flags().String("code", "", "Invite code to redeem while signing in (prefilled on the sign-in page)")
	login.AddCommand(&cobra.Command{Use: "status", Short: "Ask the kit service who this machine is signed in as", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return runLoginStatus(cmd.Context(), cmd.OutOrStdout())
	}})
	logout := &cobra.Command{Use: "logout", Short: "Remove the stored hosted API key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		local, _ := cmd.Flags().GetBool("local")
		_ = local
		return runKeyLogout(cmd.OutOrStdout())
	}}
	logout.Flags().Bool("local", false, "Forget the key on this machine even if the service cannot be reached to revoke it")
	rootCmd.AddCommand(login, logout)
}

func validKitURL(raw string) bool {
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

// kitStatusError carries the HTTP status so callers can tell a rejected key
// from an unreachable service. Its message never includes the response body.
type kitStatusError struct{ code int }

func (e kitStatusError) Error() string { return fmt.Sprintf("kit service returned HTTP %d", e.code) }

func isUnauthorized(err error) bool {
	var se kitStatusError
	return errors.Is(err, kitapi.ErrUnauthorized) || (errors.As(err, &se) && se.code == http.StatusUnauthorized)
}

func kitRequest(ctx context.Context, method, target, key string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("kit service unavailable (request failed)")
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return kitStatusError{resp.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out)
	}
	return nil
}

// loginResult is what the browser hands back on the loopback callback: an
// authorization code, or the service's reason for refusing to issue one.
type loginResult struct{ code, refused string }

// inviteRequired is the refusal a service sends when the account has not yet
// redeemed an invite code.
const inviteRequired = "invite_required"

var refusalCode = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// The callback accepts only the fresh state and never exposes a code in output.
func loginCallback(state string, results chan<- loginResult) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q := r.URL.Query()
		var result loginResult
		switch {
		case r.Method != "GET" || r.URL.Path != "/callback" || q.Get("state") != state:
		case q.Get("code") != "":
			result.code = q.Get("code")
		case refusalCode.MatchString(q.Get("error")):
			result.refused = q.Get("error")
		}
		if result == (loginResult{}) {
			http.Error(w, "invalid callback", 400)
			return
		}
		select {
		case results <- result:
			if result.refused != "" {
				fmt.Fprintln(w, "Sign-in was refused. Return to your terminal.")
				return
			}
			fmt.Fprintln(w, "Signed in. Return to your terminal.")
		default:
			http.Error(w, "callback already received", 409)
		}
	})
}

func refusedError(reason string) error {
	if reason == inviteRequired {
		return fmt.Errorf("this account needs an invite code: ask your team for one, then run hev login --code CODE")
	}
	return fmt.Errorf("sign-in refused by the kit service (%s)", reason)
}

// resolveKitService picks the service from the flag, then the environment,
// then the config file, then the hosted default.
func resolveKitService(flag string, doc map[string]any) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv(kitServiceEnv); env != "" {
		return env
	}
	kit, _ := doc["kit"].(map[string]any)
	service, _ := kit["service"].(string)
	if service != "" {
		return service
	}
	return defaultKitService
}

type kitExchange struct {
	API   string `json:"api"`
	Key   string `json:"key"`
	KeyID string `json:"key_id"`
	Team  struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"team"`
	Author struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"author"`
}

func (x kitExchange) valid() bool {
	return validKitURL(x.API) && strings.HasPrefix(x.Key, "hk_") && len(x.Key) > len("hk_") && x.KeyID != "" && x.Team.ID != "" && x.Team.Slug != "" && x.Author.ID != ""
}

func runLogin(ctx context.Context, out io.Writer, flag, invite string) error {
	if err := platformops.RequireTraceWrites(); err != nil {
		return err
	}
	for _, name := range []string{"HEVLAYER_ENDPOINT", "HEVLAYER_API_KEY"} {
		if layerenv.Get(name) != "" {
			return fmt.Errorf("unset %s before login: it would override the signed-in target", name)
		}
	}
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	if doc["invite"] != nil {
		return fmt.Errorf("leave the invite archive (hev leave) before signing in")
	}
	if kit, _ := doc["kit"].(map[string]any); kit["key"] != nil {
		return fmt.Errorf("already signed in; run hev logout first")
	}
	service := resolveKitService(flag, doc)
	if service == "" {
		return fmt.Errorf("no kit service: pass --service URL, set %s, or set service under [kit] in the config", kitServiceEnv)
	}
	if !validKitURL(service) {
		return fmt.Errorf("kit service must use HTTPS or loopback HTTP")
	}
	service = strings.TrimRight(service, "/")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	state, verifier := loginRandom(), loginRandom()
	sum := sha256.Sum256([]byte(verifier))
	results := make(chan loginResult, 1)
	server := &http.Server{Handler: loginCallback(state, results), ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go server.Serve(listener)
	u, _ := url.Parse(service + "/cli/login")
	q := u.Query()
	q.Set("state", state)
	q.Set("challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("port", fmt.Sprint(listener.Addr().(*net.TCPAddr).Port))
	if invite != "" {
		q.Set("code", invite) // the invite code; the sign-in page prefills it
	}
	u.RawQuery = q.Encode()
	fmt.Fprintln(out, "Open to sign in:", u.String())
	go openBrowser(u.String())
	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()
	var result loginResult
	select {
	case result = <-results:
	case <-ctx.Done():
		if invite == "" {
			return fmt.Errorf("sign-in timed out or canceled; if the page asked for an invite code, run hev login --code CODE")
		}
		return fmt.Errorf("sign-in timed out or canceled")
	}
	if result.refused != "" {
		return refusedError(result.refused)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	var x kitExchange
	if err = kitRequest(ctx, "POST", service+"/cli/exchange", "", map[string]string{"code": result.code, "verifier": verifier, "hostname": hostname}, &x); err != nil {
		return err
	}
	if !x.valid() {
		return fmt.Errorf("invalid response from kit service")
	}
	if x.Author.Name == "" {
		x.Author.Name = x.Author.ID
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
	kit := daemon.KitConfig{
		Service: service, API: strings.TrimRight(x.API, "/"), Key: x.Key, KeyID: x.KeyID,
		TeamID: x.Team.ID, TeamSlug: x.Team.Slug, AuthorID: x.Author.ID, AuthorName: x.Author.Name,
	}
	if err = daemon.WriteKitConfig(doc, kit); err != nil {
		return err
	}
	updateSignup(func(s *signupState) { s.Earned = nil; s.Finished = true })
	fmt.Fprintf(out, "Signed in to team %s as %s (key %s). The previous archive target is saved for hev logout.\n", x.Team.Slug, x.Author.Name, x.KeyID)
	fmt.Fprintln(out, "hev d now captures to the kit API (rows queue on this machine while it is unreachable).")
	return nil
}

// offerKitImport says how to bring this machine's history along when there
// is any.
func offerKitImport(out io.Writer) {
	cfg, err := daemon.LoadConfig()
	if err != nil || !cfg.Kit.SignedIn() {
		return
	}
	target, err := daemon.OpenTargetRead(cfg, "")
	if err != nil || target.Files == nil {
		return
	}
	rows, err := target.Files.ListSessionRows(1, nil)
	if err != nil || len(rows) == 0 {
		return
	}
	fmt.Fprintln(out, "This machine has captured sessions. Copy them and its board to the team with: hev login --import")
}

func signedInKit() (*daemon.KitConfig, error) {
	cfg, err := daemon.LoadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Kit.SignedIn() {
		return nil, fmt.Errorf("not signed in: no hosted API key configured;\n%s", daemon.HostedSetup)
	}
	return cfg.Kit, nil
}

func runLoginStatus(ctx context.Context, out io.Writer) error {
	kit, err := signedInKit()
	if err != nil {
		return err
	}
	if !validKitURL(kit.API) {
		return fmt.Errorf("invalid kit API URL in config")
	}
	var who struct {
		KeyID string `json:"key_id"`
		Team  struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"team"`
		Author struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"author"`
		Hostname string `json:"hostname"`
	}
	err = kitRequest(ctx, "GET", kit.API+"/v1/whoami", kit.Key, nil, &who)
	if isUnauthorized(err) {
		if daemon.ResolveAPIKey("") != "" {
			return fmt.Errorf("HEV_API_KEY was refused (revoked or wrong); create a key in Settings and export HEV_API_KEY again")
		}
		return fmt.Errorf("the kit service rejected the stored key; replace it with hev login")
	}
	if err != nil {
		return err
	}
	name := who.Author.Name
	if name == "" {
		name = who.Author.ID
	}
	fmt.Fprintf(out, "Signed in to team %s as %s\nMachine: %s\nKey: %s\nAPI: %s\n", who.Team.Slug, name, who.Hostname, who.KeyID, kit.API)
	if o, err := kitcapture.OpenOutbox(daemon.KitOutboxDir(), kit.TeamID); err == nil {
		pending, _ := o.Pending()
		fmt.Fprintf(out, "Outbox: %d batches queued in %s\n", len(pending), o.Dir())
	}
	// hevd's last word on hosted capture: a 402 (team read-only for
	// billing), an outage or a full outbox shows here.
	if st, err := daemon.ReadStatus(); err == nil && st.Notice != "" {
		fmt.Fprintf(out, "hevd: %s\n", st.Notice)
	}
	return nil
}

func runLogout(ctx context.Context, out io.Writer, local bool) error {
	if err := platformops.RequireTraceWrites(); err != nil {
		return err
	}
	kit, err := signedInKit()
	if err != nil {
		return err
	}
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	revoked := false
	switch {
	case !validKitURL(kit.Service):
		if !local {
			return fmt.Errorf("invalid kit service URL in config; run hev logout --local to forget the key without revoking it")
		}
	default:
		err = kitRequest(ctx, "POST", kit.Service+"/cli/logout", kit.Key, nil, nil)
		switch {
		case err == nil || isUnauthorized(err):
			revoked = true // A rejected key is already unusable.
		case !local:
			return fmt.Errorf("%w; run hev logout --local to forget the key without revoking it", err)
		}
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
	if err = daemon.ClearKitConfig(doc); err != nil {
		return err
	}
	if revoked {
		fmt.Fprintln(out, "Signed out; this machine's kit key is revoked and the previous archive target is restored.")
	} else {
		fmt.Fprintf(out, "Forgot kit key %s on this machine without revoking it; the previous archive target is restored.\n", kit.KeyID)
	}
	return nil
}

// runKeyLogin verifies before changing the credential file and retains the
// legacy writer coordination when switching the daemon target.
func runKeyLogin(ctx context.Context, out io.Writer, service, key string) error {
	if key == "" {
		return fmt.Errorf("not signed in: no hosted API key configured;\n%s", daemon.HostedSetup)
	}
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	service = strings.TrimRight(resolveKitService(service, doc), "/")
	if !validKitURL(service) {
		return fmt.Errorf("kit service must use HTTPS or loopback HTTP")
	}
	stored := ""
	if previous, ok := doc["kit"].(map[string]any); ok {
		stored, _ = previous["key"].(string)
	}
	c := kitapi.New(service, key, stored)
	c.Retries = 0
	who, err := c.Whoami(ctx)
	if err != nil {
		if isUnauthorized(err) {
			return fmt.Errorf("API key was refused (revoked or wrong); create a key in Settings and replace HEV_API_KEY")
		}
		return err
	}
	k := daemon.KitConfig{Service: service, API: service, Key: key, KeyID: who.KeyID, TeamID: who.Team.ID, TeamSlug: who.Team.Slug, AuthorID: who.Author.ID, AuthorName: who.Author.Name}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unlock, err := stopInviteWriter(ctx, home)
	if err != nil {
		return err
	}
	defer unlock()
	if err := daemon.SaveKitConfig(k); err != nil {
		return err
	}
	fmt.Fprintln(out, "API key verified and saved. Hosted capture is enabled.")
	return nil
}

func runKeyLogout(out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unlock, err := stopInviteWriter(context.Background(), home)
	if err != nil {
		return err
	}
	defer unlock()
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	if err := daemon.ClearKitConfig(doc); err != nil {
		return err
	}
	fmt.Fprintln(out, "Stored API key removed.")
	if daemon.ResolveAPIKey("") != "" {
		fmt.Fprintln(out, "Exported HEV_API_KEY still applies; unset HEV_API_KEY to stop using it.")
	}
	return nil
}
