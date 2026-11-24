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
	"github.com/hev/kit/internal/redact"
	"github.com/hev/kit/pkg/kitapi"
	"github.com/spf13/cobra"
)

// kitServiceEnv names the sign-in service when neither --service nor the
// config's [kit] service does.
const kitServiceEnv = "HEV_KIT_SERVICE"
const defaultKitService = "https://kit.hevmind.com"

// openBrowser is replaced in tests by a fake browser.
var openBrowser = func(u string) {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "linux":
		name = "xdg-open"
	default:
		return // The printed URL works on systems without a browser launcher.
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, name, u).Run()
}

// loginTimeout bounds the wait for the browser callback.
var loginTimeout = 3 * time.Minute

func init() {
	login := &cobra.Command{Use: "login", Short: "Sign this machine into Club Kit", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		service, _ := cmd.Flags().GetString("service")
		invite, _ := cmd.Flags().GetString("code")
		importHistory, _ := cmd.Flags().GetBool("import")
		if importHistory && kitSignedIn() != nil {
			// Already signed in: import (or resume importing) only.
			return runKitImport(cmd.Context(), cmd.OutOrStdout())
		}
		if err := runLogin(cmd.Context(), cmd.OutOrStdout(), service, invite); err != nil {
			return err
		}
		if importHistory {
			return runKitImport(cmd.Context(), cmd.OutOrStdout())
		}
		offerKitImport(cmd.OutOrStdout())
		return nil
	}}
	login.Flags().String("service", "", "Club Kit sign-in URL (or "+kitServiceEnv+", or [kit] service in the config)")
	login.Flags().Bool("import", false, "Copy this machine's trace archive and board to the Club Kit API (resumable; run again after signing in to resume)")
	login.Flags().String("code", "", "Invite code to redeem while signing in (prefilled on the sign-in page)")
	login.AddCommand(&cobra.Command{Use: "status", Short: "Ask Club Kit who this machine is signed in as", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return runLoginStatus(cmd.Context(), cmd.OutOrStdout())
	}})
	logout := &cobra.Command{Use: "logout", Short: "Revoke this machine's Club Kit key and restore the previous archive target", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		local, _ := cmd.Flags().GetBool("local")
		return runLogout(cmd.Context(), cmd.OutOrStdout(), local)
	}}
	logout.Flags().Bool("local", false, "Forget the key on this machine even if the service cannot be reached to revoke it")
	rootCmd.AddCommand(login, logout)
}

func validKitURL(raw string) bool {
	return kitapi.ValidBaseURL(raw)
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

func (e kitStatusError) Error() string { return fmt.Sprintf("Club Kit returned HTTP %d", e.code) }

func isUnauthorized(err error) bool {
	var se kitStatusError
	return errors.As(err, &se) && se.code == http.StatusUnauthorized
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
		return errors.New(redact.HideKitKeys(err.Error(), key))
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
		return fmt.Errorf("Club Kit unavailable: %s", redact.HideKitKeys(err.Error(), key))
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
	return errors.New("sign-in refused by Club Kit")
}

// resolveKitService picks the service from the flag, then the environment,
// then the config file.
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
	for _, name := range []string{"HEVLAYER_ENDPOINT", "HEVLAYER_API_KEY", "HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF"} {
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
	if !validKitURL(service) {
		return fmt.Errorf("Club Kit service must use HTTPS or loopback HTTP")
	}
	service = strings.TrimRight(service, "/")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	state, verifier := loginRandom(), loginRandom()
	sum := sha256.Sum256([]byte(verifier))
	results := make(chan loginResult, 1)
	server := &http.Server{Handler: loginCallback(state, results), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
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
	fmt.Fprintln(out, "Open to sign in:", redact.HideKitKeys(u.String(), ""))
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
	var who kitapi.Whoami
	if err = kitRequest(ctx, http.MethodGet, strings.TrimRight(x.API, "/")+"/v1/whoami", x.Key, nil, &who); err != nil {
		return err
	}
	if who.Team.ID != x.Team.ID || who.KeyID != x.KeyID {
		return errors.New("Club Kit sign-in identity does not match the issued key")
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
	doc, err = daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	if entry, _ := doc["kit"].(map[string]any); entry["key"] != nil || doc["invite"] != nil {
		return errors.New("the sign-in changed while the browser was open")
	}
	kit := daemon.KitConfig{
		Service: service, API: strings.TrimRight(x.API, "/"), Key: x.Key, KeyID: x.KeyID,
		TeamID: x.Team.ID, TeamSlug: x.Team.Slug, AuthorID: x.Author.ID, AuthorName: x.Author.Name,
	}
	if err = daemon.WriteKitConfig(doc, kit); err != nil {
		return err
	}
	fmt.Fprint(out, redact.HideKitKeys(fmt.Sprintf("Signed in to Club Kit team %s as %s (key %s). The previous archive target is saved for hev logout.\n", x.Team.Slug, x.Author.Name, x.KeyID), x.Key))
	fmt.Fprintln(out, "Restart capture with hev up or hev d to queue rows for the Club Kit API.")
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
	kit, err := daemon.ReadKitConfig()
	if err != nil {
		return nil, err
	}
	if !kit.SignedIn() {
		return nil, fmt.Errorf("not signed in; run hev login")
	}
	return kit, nil
}

func runLoginStatus(ctx context.Context, out io.Writer) error {
	kit, err := signedInKit()
	if err != nil {
		return err
	}
	if !validKitURL(kit.API) {
		return fmt.Errorf("invalid Club Kit API URL in config")
	}
	var who kitapi.Whoami
	err = kitRequest(ctx, "GET", kit.API+"/v1/whoami", kit.Key, nil, &who)
	if isUnauthorized(err) {
		return fmt.Errorf("Club Kit rejected this machine's key %s (revoked or expired); run hev logout --local, then hev login", redact.HideKitKeys(kit.KeyID, kit.Key))
	}
	if err != nil {
		return err
	}
	if kit.TeamID != "" && who.Team.ID != kit.TeamID {
		return kitcapture.ErrTeamMismatch
	}
	name := who.Author.Name
	if name == "" {
		name = who.Author.ID
	}
	fmt.Fprint(out, redact.HideKitKeys(fmt.Sprintf("Signed in to Club Kit team %s as %s\nMachine: %s\nKey: %s\nAPI: %s\n", who.Team.Slug, name, who.Hostname, who.KeyID, kit.API), kit.Key))
	if o, err := kitcapture.OpenOutbox(daemon.KitOutboxDir(), kit.TeamID); err == nil {
		pending, _ := o.Pending()
		fmt.Fprintf(out, "Outbox: %d batches queued in %s\n", len(pending), o.Dir())
	}
	// hevd's last word on hosted capture: a 402 (team read-only for
	// billing), an outage or a full outbox shows here.
	if st, err := daemon.ReadStatus(); err == nil && st.Notice != "" {
		fmt.Fprintf(out, "hevd: %s\n", redact.HideKitKeys(st.Notice, kit.Key))
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
	revoked := false
	switch {
	case !validKitURL(kit.Service):
		if !local {
			return fmt.Errorf("invalid Club Kit service URL in config; run hev logout --local to forget the key without revoking it")
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
	doc, err := daemon.ReadInviteConfig()
	if err != nil {
		return err
	}
	if entry, _ := doc["kit"].(map[string]any); entry["key"] != kit.Key {
		return errors.New("the Club Kit sign-in changed during logout")
	}
	if err = daemon.ClearKitConfig(doc); err != nil {
		return err
	}
	if revoked {
		fmt.Fprintln(out, "Signed out of Club Kit; this machine's key is revoked and the previous archive target is restored.")
	} else {
		fmt.Fprintln(out, "Forgot this machine's Club Kit key. The previous archive target is restored.")
	}
	return nil
}
