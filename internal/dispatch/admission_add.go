package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hev/kit/internal/version"
)

// Teammate repository additions are bounded; an owner can always clone by hand.
var (
	addRepoTimeout  = 10 * time.Minute
	addRepoMaxBytes = int64(4 << 30)
)

// repoCloner is replaced in tests that exercise admission without a server.
var repoCloner = safeClone

// RepoCapability is the grant capability that allows "admission-add" and
// "admission-remove".
const RepoCapability = "repos"

// AdmittedRepo is one [admission] repos entry as shown by "hev admission ls".
type AdmittedRepo struct {
	Origin  string    `json:"origin"`
	Name    string    `json:"name,omitempty"`
	AddedBy string    `json:"added_by,omitempty"`
	AddedAt time.Time `json:"added_at,omitempty"`
}

// RepoPolicy is the host's repository admission as seen by one caller.
type RepoPolicy struct {
	// AdmitAll means [admission] repos is absent: every workspace repo is admitted.
	AdmitAll bool           `json:"admit_all"`
	Repos    []AdmittedRepo `json:"repos"`
	AddFrom  []string       `json:"add_from"`
	CanAdd   bool           `json:"can_add"`
}

// RepoCaller is who asks. Restricted callers need the grant capability and an
// add_from match; owners (local or unrestricted SSH) need neither.
type RepoCaller struct {
	Sender     string
	Restricted bool
	// CanAdd is the repos capability; it covers both add and remove.
	CanAdd bool
}

type addURL struct {
	Clone    string // canonical URL handed to git and recorded in admission
	Identity string // repoIdentity of Clone
	Name     string // workspace directory name
	// Transport is "https" or "ssh" (URL or scp-like); User is the SSH user.
	Transport string
	User      string
}

var (
	urlHost    = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	urlUser    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
	urlPort    = regexp.MustCompile(`^[0-9]{1,5}$`)
	urlSegment = regexp.MustCompile(`^[A-Za-z0-9_~@ +-][A-Za-z0-9._~@ +-]*$`)
	repoDir    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
)

// parseAddURL accepts only https and ssh remotes (URL or scp-like form) with a
// plain host and path. Everything git could interpret as a local path, helper
// transport, option, credential or traversal is refused rather than rewritten.
func parseAddURL(raw string, needRepo bool) (addURL, error) {
	var a addURL
	refuse := func(why string) (addURL, error) {
		return a, fmt.Errorf("refused repository URL %q: %s", raw, why)
	}
	if raw == "" || len(raw) > 512 {
		return refuse("empty or too long")
	}
	for _, c := range raw {
		if c <= ' ' || c == 0x7f || c > 0x7e {
			return refuse("whitespace, control or non-ASCII character (write spaces as %20)")
		}
	}
	if strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, "\\\"'`$?#") || strings.Contains(raw, "::") {
		return refuse("option, quoting, query, fragment or helper transport syntax")
	}
	if strings.Contains(strings.ReplaceAll(raw, "%20", ""), "%") {
		return refuse("percent-encoding other than %20")
	}
	var scheme, user, host, port, p string
	if i := strings.Index(raw, "://"); i >= 0 {
		u, err := url.Parse(raw)
		if err != nil {
			return refuse(err.Error())
		}
		scheme = strings.ToLower(u.Scheme)
		if scheme != "https" && scheme != "ssh" {
			return refuse("only https:// and ssh:// remotes can be added")
		}
		if u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
			return refuse("unexpected URL form")
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); scheme == "https" || hasPassword {
				return refuse("credentials or user names in URLs are not accepted; the host uses its own git credentials")
			}
			user = u.User.Username()
		}
		host, port, p = u.Hostname(), u.Port(), u.Path
		if strings.Contains(u.Host, "[") || (port == "" && strings.HasSuffix(u.Host, ":")) {
			return refuse("unsupported host")
		}
	} else {
		colon := strings.IndexByte(raw, ':')
		if colon < 0 || strings.Contains(raw[:colon], "/") {
			return refuse("local paths and file remotes cannot be added")
		}
		scheme, host, p = "scp", raw[:colon], raw[colon+1:]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			user, host = host[:at], host[at+1:]
		}
		if strings.ContainsAny(p, ":%") {
			return refuse("unexpected colon or percent-encoding in scp-like path")
		}
	}
	host = strings.ToLower(host)
	// One-letter hosts are Windows drive letters to git.
	if len(host) < 2 || !urlHost.MatchString(host) || strings.Contains(host, "..") {
		return refuse("invalid host")
	}
	if user != "" && !urlUser.MatchString(user) {
		return refuse("invalid SSH user")
	}
	if port != "" && !urlPort.MatchString(port) {
		return refuse("invalid port")
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
	segments := []string{}
	if trimmed != "" {
		segments = strings.Split(trimmed, "/")
	}
	for _, s := range segments {
		if s == "." || s == ".." || !urlSegment.MatchString(s) {
			return refuse("invalid or traversing path segment")
		}
	}
	if needRepo && len(segments) == 0 {
		return refuse("no repository path")
	}
	hostport := host
	if port != "" {
		hostport += ":" + port
	}
	userPrefix := ""
	if user != "" {
		userPrefix = user + "@"
	}
	escaped := strings.ReplaceAll(strings.Join(segments, "/"), " ", "%20")
	switch scheme {
	case "scp":
		a.Clone = userPrefix + host + ":" + escaped
	default:
		a.Clone = scheme + "://" + userPrefix + hostport + "/" + escaped
	}
	a.Identity = repoIdentity(a.Clone)
	a.Transport, a.User = scheme, user
	if scheme == "scp" {
		a.Transport = "ssh"
	}
	if len(segments) > 0 {
		a.Name = strings.TrimSuffix(segments[len(segments)-1], ".git")
		a.Name = strings.ReplaceAll(a.Name, " ", "-")
	}
	if needRepo && (!repoDir.MatchString(a.Name) || strings.HasSuffix(a.Name, ".") || windowsReserved(a.Name)) {
		return refuse("repository name cannot be used as a workspace directory")
	}
	return a, nil
}

func windowsReserved(name string) bool {
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9'
}

// addFromAllows matches whole path segments of normalized identities, so an
// entry for an organization never admits a longer sibling name, and requires
// at least one segment beyond the entry: the organization itself is not a
// repository. The transport (and SSH user) must match the entry's too, so an
// https:// entry, which the host serves with its HTTPS credential, never
// admits an ssh:// or scp-like URL that would use its SSH keys, and the
// reverse.
func addFromAllows(addFrom []string, candidate addURL) bool {
	for _, entry := range addFrom {
		prefix, err := parseAddURL(strings.TrimSpace(entry), false)
		if err != nil {
			continue
		}
		if prefix.Transport != candidate.Transport || prefix.User != candidate.User {
			continue
		}
		p := strings.TrimSuffix(prefix.Identity, "/")
		if rest, ok := strings.CutPrefix(candidate.Identity, p+"/"); ok && strings.Trim(rest, "/") != "" {
			return true
		}
	}
	return false
}

// AddRepo validates, clones with the host's own git configuration and admits a
// repository. It is idempotent: an existing clone or admission entry is reused.
func AddRepo(ctx context.Context, raw string, caller RepoCaller) (Response, error) {
	r := Response{}
	if caller.Sender == "" {
		return r, fmt.Errorf("authenticated sender required")
	}
	if caller.Restricted && !caller.CanAdd {
		return r, fmt.Errorf("permission denied: your access does not include adding repos (the owner can run: hev nodes allow NAME %s)", RepoCapability)
	}
	a, err := parseAddURL(strings.TrimSpace(raw), true)
	if err != nil {
		return r, err
	}
	cfgPath := ConfigPath("")
	policy, err := targetPolicy()
	if err != nil {
		return r, fmt.Errorf("target admission config: %w", err)
	}
	if caller.Restricted && !addFromAllows(policy.Admission.AddFrom, a) {
		return r, fmt.Errorf("permission denied: %s is not in an allowed organization (no [admission] add_from prefix on this host matches)", a.Clone)
	}
	workspace := Expand(policy.Workspace)
	if err = os.MkdirAll(workspace, 0700); err != nil {
		return r, err
	}
	dest := filepath.Join(workspace, a.Name)
	cloned, err := ensureClone(ctx, a, workspace, dest)
	var ce *cloneError
	if errors.As(err, &ce) && caller.Restricted && ce.Detail != "" {
		// git's output can echo host paths, config and credential helper
		// messages; the teammate gets the summary and the owner the log.
		logCloneFailure(caller.Sender, a.Clone, ce.Detail)
		return r, fmt.Errorf("%s for %s; the owner can find details in %s on the host", ce.Summary, a.Clone, filepath.Base(cloneLogPath()))
	}
	if err != nil {
		return r, err
	}
	admitted, err := admitAddedRepo(cfgPath, a, caller.Sender, cloned)
	if err != nil {
		return r, err
	}
	switch {
	case cloned && admitted:
		r.Message = fmt.Sprintf("added %s as %s", a.Clone, a.Name)
	case admitted:
		r.Message = fmt.Sprintf("admitted %s; existing clone %s", a.Clone, a.Name)
	case cloned:
		r.Message = fmt.Sprintf("cloned %s as %s; already admitted", a.Clone, a.Name)
	default:
		r.Message = fmt.Sprintf("%s already present as %s", a.Clone, a.Name)
	}
	r.Message += "; dispatch with repo: " + a.Name
	return r, nil
}

// ensureClone reports whether it cloned. A same-origin clone is reused and a
// different repository under the same name is never replaced.
func ensureClone(ctx context.Context, a addURL, workspace, dest string) (bool, error) {
	same := func() (bool, error) {
		origin, err := command("", "git", "-C", dest, "remote", "get-url", "origin")
		if err != nil {
			return false, fmt.Errorf("workspace already has %s, which is not a clone with an origin; ask the owner", filepath.Base(dest))
		}
		if repoIdentity(origin) != a.Identity {
			return false, fmt.Errorf("workspace already has %s from a different origin; ask the owner", filepath.Base(dest))
		}
		return false, nil
	}
	if _, err := os.Lstat(dest); err == nil {
		return same()
	}
	tmp, err := os.MkdirTemp(workspace, ".hev-admission-add-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	if err = repoCloner(ctx, a.Clone, filepath.Join(tmp, "repo")); err != nil {
		return false, err
	}
	if err = os.Rename(filepath.Join(tmp, "repo"), dest); err != nil {
		if _, statErr := os.Lstat(dest); statErr == nil {
			// A concurrent add won the rename.
			return same()
		}
		return false, err
	}
	return true, nil
}

// cloneWaitDelay bounds how long git's helpers may hold its output open after
// git itself has exited or been killed.
var cloneWaitDelay = 5 * time.Second

// cloneError keeps git's output out of restricted callers' replies; Detail is
// logged on the host instead.
type cloneError struct {
	Summary string
	Detail  string
}

func (e *cloneError) Error() string {
	if e.Detail == "" {
		return e.Summary
	}
	return e.Summary + ": " + e.Detail
}

// safeClone runs git with transport, hook, submodule, LFS and redirect
// hardening, in its own process group (Unix) or job (Windows) so cancellation
// also reaches transport and credential helpers. Only the URL and a
// host-chosen destination come from the request.
func safeClone(ctx context.Context, cloneURL, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, addRepoTimeout)
	defer cancel()
	hooks, err := os.MkdirTemp("", "hev-no-hooks-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(hooks)
	args := []string{
		"-c", "core.hooksPath=" + filepath.ToSlash(hooks),
		"-c", "protocol.allow=never",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.ssh.allow=always",
		// The URL-scoped key outranks any http.<url>.followRedirects in host config.
		"-c", "http.followRedirects=false",
		"-c", "http." + cloneURL + ".followRedirects=false",
		"-c", "submodule.recurse=false",
		"-c", "core.fsmonitor=false",
		"-c", "core.askPass=",
		// No LFS during the clone: a repository's .lfsconfig cannot redirect
		// object fetches or credential lookups.
		"-c", "filter.lfs.process=",
		"-c", "filter.lfs.smudge=",
		"-c", "filter.lfs.required=false",
		"clone", "--quiet", "--no-recurse-submodules", "--template=", "--", cloneURL, dest,
	}
	c := helperCommand(ctx, "git", args...)
	// GIT_ALLOW_PROTOCOL overrides any host protocol.<name>.allow setting,
	// including for submodules and alternates.
	c.Env = append(withoutGitRepoEnv(c.Env), "GIT_ALLOW_PROTOCOL=https:ssh", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_PROTOCOL_FROM_USER=0", "GIT_LFS_SKIP_SMUDGE=1", "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	c.WaitDelay = cloneWaitDelay
	var tooLarge atomic.Bool
	watchDone := make(chan struct{})
	release, err := startSourceChild(c, "")
	if err != nil {
		return &cloneError{Summary: "git clone could not start", Detail: err.Error()}
	}
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-t.C:
				if dirSize(dest) > addRepoMaxBytes {
					tooLarge.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	err = c.Wait()
	close(watchDone)
	// Nothing git started outlives the clone, even after a clean exit.
	release()
	if tooLarge.Load() || (err == nil && dirSize(dest) > addRepoMaxBytes) {
		return &cloneError{Summary: fmt.Sprintf("clone exceeded %d MiB; ask the owner to clone it", addRepoMaxBytes>>20)}
	}
	if ctx.Err() == context.DeadlineExceeded {
		return &cloneError{Summary: fmt.Sprintf("clone exceeded %s; ask the owner to clone it", addRepoTimeout)}
	}
	if err != nil {
		return &cloneError{Summary: "git clone failed", Detail: strings.TrimSpace(fmt.Sprintf("%v: %s", err, out.String()))}
	}
	return nil
}

func cloneLogPath() string { return filepath.Join(Root(), "admission-clone.log") }

// logCloneFailure keeps a teammate's failed clone output on the host only.
func logCloneFailure(sender, cloneURL, detail string) {
	if os.MkdirAll(Root(), 0700) != nil {
		return
	}
	f, err := os.OpenFile(cloneLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s:\n%s\n", time.Now().UTC().Format(time.RFC3339), sender, cloneURL, detail)
}

// withoutGitRepoEnv drops variables that would redirect git to another repository.
func withoutGitRepoEnv(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_TEMPLATE_DIR", "GIT_TERMINAL_PROMPT", "GIT_ALLOW_PROTOCOL", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT":
			continue
		case "GIT_ASKPASS", "SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", "GIT_SSH_COMMAND":
			// A prompt must fail rather than wait for someone to answer it.
			continue
		}
		out = append(out, kv)
	}
	return out
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// The admission log records who added or removed each entry, next to the config.
type repoLedgerEntry struct {
	// Action is "remove" for removals; additions leave it empty.
	Action  string    `json:"action,omitempty"`
	Origin  string    `json:"origin"`
	Name    string    `json:"name"`
	AddedBy string    `json:"added_by,omitempty"`
	AddedAt time.Time `json:"added_at,omitzero"`
	Cloned  bool      `json:"cloned,omitempty"`
	// RemovedBy and RemovedAt attribute a removal.
	RemovedBy string    `json:"removed_by,omitempty"`
	RemovedAt time.Time `json:"removed_at,omitzero"`
}

func repoLedgerPath(cfgPath string) string {
	return filepath.Join(filepath.Dir(cfgPath), "admission-log.jsonl")
}

func readRepoLedger(cfgPath string) []repoLedgerEntry {
	b, err := os.ReadFile(repoLedgerPath(cfgPath))
	if err != nil {
		return nil
	}
	out := []repoLedgerEntry{}
	for _, line := range strings.Split(string(b), "\n") {
		var e repoLedgerEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.Origin != "" {
			out = append(out, e)
		}
	}
	return out
}

func appendRepoLedger(cfgPath string, e repoLedgerEntry) error {
	entry, _ := json.Marshal(e)
	f, err := os.OpenFile(repoLedgerPath(cfgPath), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(entry, '\n'))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// admitAddedRepo appends the origin to [admission] repos under the dispatch
// lock. An absent list admits every repo already, so it is left absent.
func admitAddedRepo(cfgPath string, a addURL, sender string, cloned bool) (bool, error) {
	if err := os.MkdirAll(Root(), 0700); err != nil {
		return false, err
	}
	unlock, err := lockRoot()
	if err != nil {
		return false, err
	}
	defer unlock()
	text, err := os.ReadFile(cfgPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var current struct {
		Admission Admission `toml:"admission"`
	}
	if _, err = toml.Decode(string(text), &current); err != nil {
		return false, fmt.Errorf("target admission config: %w", err)
	}
	if current.Admission.Repos == nil {
		return false, nil
	}
	for _, existing := range current.Admission.Repos {
		if repoIdentity(existing) == a.Identity {
			return false, nil
		}
	}
	edited, err := appendAdmissionRepo(string(text), a.Clone)
	if err != nil {
		return false, err
	}
	if err = writeFileAtomic(cfgPath, []byte(edited)); err != nil {
		return false, err
	}
	if err = appendRepoLedger(cfgPath, repoLedgerEntry{Origin: a.Clone, Name: a.Name, AddedBy: sender, AddedAt: time.Now().UTC(), Cloned: cloned}); err != nil {
		return true, fmt.Errorf("admitted %s but could not record attribution: %w", a.Clone, err)
	}
	return true, nil
}

// writeFileAtomic replaces path with a fully written sibling, keeping its mode.
func writeFileAtomic(path string, b []byte) error {
	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hev-config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// appendAdmissionRepo inserts one string into the [admission] repos array of a
// TOML document, leaving every other byte in place, and proves by decoding that
// the only semantic change is that appended element.
func appendAdmissionRepo(text, origin string) (string, error) {
	open, close, err := findAdmissionRepos(text)
	if err != nil {
		return "", err
	}
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	item := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(origin) + `"`
	last := lastSignificant(text, open, close)
	var edited string
	if !strings.Contains(text[last:close], "\n") {
		// The closing bracket shares a line with the last element: insert inline.
		sep := ", "
		if last == open {
			sep = ""
		} else if text[last] == ',' {
			sep = " "
		}
		edited = text[:last+1] + sep + item + text[last+1:]
	} else {
		lineStart := strings.LastIndexByte(text[:last+1], '\n') + 1
		indent := text[lineStart : lineStart+len(text[lineStart:])-len(strings.TrimLeft(text[lineStart:], " \t"))]
		comma, trailing := "", ""
		switch {
		case last == open:
			keyStart := strings.LastIndexByte(text[:open], '\n') + 1
			indent = text[keyStart:keyStart+len(text[keyStart:])-len(strings.TrimLeft(text[keyStart:], " \t"))] + "  "
			trailing = ","
		case text[last] == ',':
			trailing = ","
		default:
			comma = ","
		}
		eol := last + 1 + strings.IndexByte(text[last+1:], '\n')
		if eol > 0 && text[eol-1] == '\r' {
			eol--
		}
		edited = text[:last+1] + comma + text[last+1:eol] + nl + indent + item + trailing + text[eol:]
	}
	if err = sameExceptAppended(text, edited, origin); err != nil {
		return "", err
	}
	return edited, nil
}

func sameExceptAppended(before, after, origin string) error {
	refuse := fmt.Errorf("cannot safely edit [admission] repos in the target config; add %q by hand", origin)
	var a, b map[string]any
	if _, err := toml.Decode(before, &a); err != nil {
		return refuse
	}
	if _, err := toml.Decode(after, &b); err != nil {
		return refuse
	}
	admission, ok := b["admission"].(map[string]any)
	if !ok {
		return refuse
	}
	repos, ok := admission["repos"].([]any)
	if !ok || len(repos) == 0 || repos[len(repos)-1] != origin {
		return refuse
	}
	admission["repos"] = repos[:len(repos)-1]
	if !reflect.DeepEqual(a, b) {
		return refuse
	}
	return nil
}

// findAdmissionRepos locates the brackets of "repos = [...]" directly inside
// an [admission] table header. Other spellings (dotted keys, inline tables)
// are left to the owner rather than guessed at.
func findAdmissionRepos(text string) (int, int, error) {
	notFound := fmt.Errorf("cannot find a repos = [...] array under an [admission] table in the target config")
	inAdmission := false
	for i := 0; i < len(text); {
		lineEnd := strings.IndexByte(text[i:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += i
		}
		line := strings.TrimLeft(text[i:lineEnd], " \t")
		lineAt := lineEnd - len(line)
		if strings.HasPrefix(line, "[") {
			end := strings.IndexByte(line, ']')
			inAdmission = end > 0 && strings.TrimSpace(line[1:end]) == "admission" && !strings.HasPrefix(line, "[[")
		} else if inAdmission && strings.HasPrefix(line, "repos") {
			rest := strings.TrimLeft(line[len("repos"):], " \t")
			if strings.HasPrefix(rest, "=") {
				value := strings.TrimLeft(rest[1:], " \t")
				if !strings.HasPrefix(value, "[") {
					return 0, 0, notFound
				}
				open := lineAt + len(line) - len(value)
				close, err := matchBracket(text, open)
				if err != nil {
					return 0, 0, err
				}
				return open, close, nil
			}
		}
		// Skip any value spanning lines so its contents are never read as keys.
		next, err := skipValueLines(text, i, lineEnd)
		if err != nil {
			return 0, 0, err
		}
		i = next
	}
	return 0, 0, notFound
}

// skipValueLines returns the start of the next logical line, stepping over
// multi-line arrays and strings that begin on the line at start.
func skipValueLines(text string, start, lineEnd int) (int, error) {
	for j := start; j < lineEnd; j++ {
		switch {
		case text[j] == '#':
			return lineEnd + 1, nil
		case strings.HasPrefix(text[j:], `"""`) || strings.HasPrefix(text[j:], `'''`):
			end, err := skipString(text, j)
			if err != nil {
				return 0, err
			}
			if end > lineEnd {
				return skipValueLines(text, end, lineEndAt(text, end))
			}
			j = end - 1
		case text[j] == '"' || text[j] == '\'':
			end, err := skipString(text, j)
			if err != nil {
				return 0, err
			}
			j = end - 1
		case text[j] == '[' && strings.TrimSpace(text[start:j]) != "" && !strings.HasPrefix(strings.TrimLeft(text[start:j], " \t"), "["):
			end, err := matchBracket(text, j)
			if err != nil {
				return 0, err
			}
			if end > lineEnd {
				return skipValueLines(text, end+1, lineEndAt(text, end))
			}
			j = end
		}
	}
	return lineEnd + 1, nil
}

func lineEndAt(text string, i int) int {
	if n := strings.IndexByte(text[i:], '\n'); n >= 0 {
		return i + n
	}
	return len(text)
}

// skipString returns the index just past the string literal starting at i.
func skipString(text string, i int) (int, error) {
	for _, delim := range []string{`"""`, `'''`} {
		if strings.HasPrefix(text[i:], delim) {
			j := i + 3
			for {
				n := strings.Index(text[j:], delim)
				if n < 0 {
					return 0, fmt.Errorf("unterminated string in target config")
				}
				j += n
				if delim == `"""` && escaped(text, j) {
					j++
					continue
				}
				// Up to two quotes may directly precede the closing delimiter.
				for j+3 < len(text) && text[j+3] == delim[0] {
					j++
				}
				return j + 3, nil
			}
		}
	}
	q := text[i]
	for j := i + 1; j < len(text); j++ {
		switch {
		case text[j] == '\n':
			return 0, fmt.Errorf("unterminated string in target config")
		case text[j] == q && (q == '\'' || !escaped(text, j)):
			return j + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated string in target config")
}

// escaped reports whether the quote at j follows an odd run of backslashes.
func escaped(text string, j int) bool {
	n := 0
	for k := j - 1; k >= 0 && text[k] == '\\'; k-- {
		n++
	}
	return n%2 == 1
}

// matchBracket returns the index of the ']' closing the array opened at open.
func matchBracket(text string, open int) (int, error) {
	depth := 0
	for j := open; j < len(text); j++ {
		switch text[j] {
		case '#':
			j = lineEndAt(text, j) - 1
		case '"', '\'':
			end, err := skipString(text, j)
			if err != nil {
				return 0, err
			}
			j = end - 1
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				return j, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated array in target config")
}

// lastSignificant is the index of the last element or comma before close,
// skipping whitespace and comments; open when the array is empty.
func lastSignificant(text string, open, close int) int {
	last := open
	for j := open + 1; j < close; j++ {
		switch c := text[j]; {
		case c == '#':
			j = lineEndAt(text, j) - 1
		case c == '"' || c == '\'':
			end, _ := skipString(text, j)
			last = end - 1
			j = end - 1
		case c == '[' || c == '{':
			end, _ := matchBracket(text, j)
			last = end
			j = end
		case c != ' ' && c != '\t' && c != '\r' && c != '\n':
			last = j
		}
	}
	return last
}

// ListRepos reports the host's admission without changing anything.
func ListRepos(caller RepoCaller) (Response, error) {
	r := Response{}
	cfgPath := ConfigPath("")
	policy, err := targetPolicy()
	if err != nil {
		return r, fmt.Errorf("target admission config: %w", err)
	}
	p := &RepoPolicy{AdmitAll: policy.Admission.Repos == nil, Repos: []AdmittedRepo{}, AddFrom: policy.Admission.AddFrom, CanAdd: !caller.Restricted || (caller.CanAdd && len(policy.Admission.AddFrom) > 0)}
	if p.AddFrom == nil {
		p.AddFrom = []string{}
	}
	ledger := readRepoLedger(cfgPath)
	workspace := Expand(policy.Workspace)
	for _, origin := range policy.Admission.Repos {
		row := AdmittedRepo{Origin: origin}
		id := repoIdentity(origin)
		for _, e := range ledger {
			if repoIdentity(e.Origin) == id {
				row.AddedBy, row.AddedAt = e.AddedBy, e.AddedAt
			}
		}
		if a, err := parseAddURL(origin, true); err == nil {
			if got, err := command("", "git", "-C", filepath.Join(workspace, a.Name), "remote", "get-url", "origin"); err == nil && repoIdentity(got) == id {
				row.Name = a.Name
			}
		}
		p.Repos = append(p.Repos, row)
	}
	r.RepoPolicy = p
	return r, nil
}

func handleRepos(q Request, caller RepoCaller) (Response, error) {
	var r Response
	var err error
	switch q.Action {
	case "admission-add":
		r, err = AddRepo(context.Background(), q.URL, caller)
	case "admission-remove":
		r, err = RemoveRepo(q.URL, caller)
	default:
		r, err = ListRepos(caller)
	}
	r.Version = version.Version
	return r, err
}
