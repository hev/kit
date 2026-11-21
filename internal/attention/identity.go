package attention

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// Person is who a notice is for: requested_by in the event.
type Person struct {
	// Person is the hosted (Clerk) user ID when this host is signed in to a
	// kit service, else the run's sender: local:ACCOUNT or a nodes teammate
	// name.
	Person string `json:"person"`
	// Name is a display name: the signed-in author's name, else the sender.
	Name string `json:"name,omitempty"`
}

// Signed is what /v1/whoami says about this host's kit key.
type Signed struct {
	ID   string
	Name string
}

// whoami reports who this host is signed in as; ok is false when it is not
// signed in or the answer is unavailable. Tests replace it.
var whoami = configWhoami

// whoamiTimeout bounds the one /v1/whoami call a process makes per cache
// period; a notice never waits longer than this for its requester.
const whoamiTimeout = 3 * time.Second

var (
	whoMu    sync.Mutex
	whoKey   string
	whoUntil time.Time
	whoVal   Signed
	whoOK    bool
)

// ResetIdentity drops the cached /v1/whoami answer.
func ResetIdentity() {
	whoMu.Lock()
	whoKey, whoUntil, whoVal, whoOK = "", time.Time{}, Signed{}, false
	whoMu.Unlock()
}

// kitSignIn is the [kit] table `hev login` writes to config.toml. author_id
// and author_name cache the last /v1/whoami answer.
type kitSignIn struct {
	Service    string `toml:"service"`
	API        string `toml:"api"`
	Key        string `toml:"key"`
	AuthorID   string `toml:"author_id"`
	AuthorName string `toml:"author_name"`
}

func readSignIn() kitSignIn {
	var f struct {
		Kit kitSignIn `toml:"kit"`
	}
	if info, err := os.Stat(ConfigPath()); err == nil && info.Mode().IsRegular() {
		_, _ = toml.DecodeFile(ConfigPath(), &f)
	}
	k := f.Kit
	// HEV_API_KEY is the primary hosted credential, as for every other
	// hosted call.
	if env := strings.TrimSpace(os.Getenv("HEV_API_KEY")); env != "" {
		k.Key = env
		if s := strings.TrimSpace(os.Getenv("HEV_KIT_SERVICE")); s != "" {
			k.Service, k.API = s, s
		}
	}
	if k.API == "" {
		k.API = k.Service
	}
	if k.API == "" {
		k.API = "https://kit.hevmind.com"
	}
	k.API = strings.TrimRight(k.API, "/")
	return k
}

// HostedCredentials returns the configured Kit API endpoint and bearer key.
func HostedCredentials() (endpoint, key string) {
	k := readSignIn()
	return k.API, k.Key
}

// configWhoami asks the kit service who this host's key belongs to. A host
// without a key is logged out. A failed call falls back to the author the
// last sign-in recorded, so a brief outage does not change who notices are
// for. Answers are cached per key for ten minutes (a minute when unknown).
func configWhoami() (Signed, bool) {
	k := readSignIn()
	if k.Key == "" {
		return Signed{}, false
	}
	sum := sha256.Sum256([]byte(k.API + "\x00" + k.Key))
	cacheKey := string(sum[:])
	whoMu.Lock()
	defer whoMu.Unlock()
	if whoKey == cacheKey && time.Now().Before(whoUntil) {
		return whoVal, whoOK
	}
	s, err := fetchWhoami(k.API, k.Key)
	ttl := 10 * time.Minute
	if err != nil {
		s = Signed{ID: k.AuthorID, Name: k.AuthorName}
		ttl = time.Minute
	}
	whoKey, whoUntil, whoVal, whoOK = cacheKey, time.Now().Add(ttl), s, s.ID != ""
	return whoVal, whoOK
}

func fetchWhoami(api, key string) (Signed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), whoamiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", api+"/v1/whoami", nil)
	if err != nil {
		return Signed{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	// Never follow a redirect: it would replay the key elsewhere.
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return Signed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Signed{}, fmt.Errorf("whoami: HTTP %d", resp.StatusCode)
	}
	var w struct {
		Author struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"author"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&w); err != nil {
		return Signed{}, err
	}
	if w.Author.ID == "" {
		return Signed{}, errors.New("whoami: no author")
	}
	return Signed{ID: w.Author.ID, Name: w.Author.Name}, nil
}

// LocalSender is this account's dispatch sender, local:ACCOUNT.
func LocalSender() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return "local:" + u.Username
	}
	return "local:unknown"
}

// Requester resolves requested_by for a run's sender (its requested_by, else
// sender; "" is this account). A local sender on a signed-in host is the
// signed-in person's Clerk user ID; a teammate stays the teammate name, and a
// logged-out host keeps the sender.
func Requester(sender string) Person {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		sender = LocalSender()
	}
	if strings.HasPrefix(sender, "local:") {
		if s, ok := whoami(); ok {
			name := s.Name
			if name == "" {
				name = sender
			}
			return Person{Person: s.ID, Name: OneLine(name, personLimit)}
		}
	}
	return Person{Person: sender, Name: sender}
}

// For sets who the notice is for: requested_by from sender (see Requester),
// and addressee defaulting to the requester.
func (n Notice) For(sender string) Notice {
	p := Requester(sender)
	n.RequestedBy = &p
	if len(n.Addressee) == 0 {
		n.Addressee = []string{p.Person}
	}
	return n
}

// attributed fills requested_by (this account) and addressee on a notice
// that arrived without them, such as one from an older host.
func (n Notice) attributed() Notice {
	if n.RequestedBy == nil || n.RequestedBy.Person == "" {
		return n.For("")
	}
	if len(n.Addressee) == 0 {
		n.Addressee = []string{n.RequestedBy.Person}
	}
	return n
}
