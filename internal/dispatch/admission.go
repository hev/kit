package dispatch

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// Admission leaves room for other resource kinds without mixing their identities.
type Admission struct {
	Repos  []string `toml:"repos"`
	Owners []string `toml:"owners"`
	// AddFrom lists origin prefixes that teammates granted the repos
	// capability may clone and admit. Absent or empty: nobody can self-add.
	AddFrom []string `toml:"add_from"`
}

// repoIdentity compares origin locations, never the sender's repo label.
// Transport and SSH user do not identify a repository. URL path case does.
func repoIdentity(remote string) string {
	remote = strings.TrimSpace(remote)
	if trace.WindowsPath(remote) || strings.HasPrefix(remote, "//") {
		remote = strings.ReplaceAll(remote, `\`, "/")
		return strings.TrimSuffix(strings.ToLower(path.Clean(remote)), ".git")
	}
	if !strings.Contains(remote, "://") && !strings.HasPrefix(remote, "/") {
		if colon := strings.IndexByte(remote, ':'); colon >= 0 {
			remote = "ssh://" + remote[:colon] + "/" + remote[colon+1:]
		}
	}
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		return strings.ToLower(u.Host) + "/" + strings.TrimSuffix(strings.Trim(path.Clean(u.Path), "/"), ".git")
	}
	if runtime.GOOS == "windows" {
		remote = strings.ToLower(remote)
	}
	return strings.TrimSuffix(path.Clean(remote), ".git")
}

func (policy Admission) admitAll() bool {
	return policy.Repos == nil && policy.Owners == nil
}

func ownerIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	if slash := strings.IndexByte(raw, '/'); slash >= 0 && !strings.Contains(raw, "://") &&
		(!strings.Contains(raw[:slash], ":") || urlPort.MatchString(raw[strings.LastIndexByte(raw[:slash], ':')+1:slash])) {
		raw = "https://" + raw
	}
	owner, err := parseAddURL(raw, false)
	if err != nil {
		return ""
	}
	// The .git suffix convention applies only to repository names.
	identity := owner.Identity
	if strings.HasSuffix(owner.Clone, ".git") {
		identity += ".git"
	}
	_, ownerPath, _ := strings.Cut(identity, "/")
	if ownerPath == "" || strings.Contains(ownerPath, "/") {
		return ""
	}
	return identity
}

func (policy Admission) admitsOwner(origin string) bool {
	if len(policy.Owners) == 0 {
		return false
	}
	// Validate before identity normalization can collapse traversal or empty segments.
	candidate, err := parseAddURL(strings.TrimSpace(origin), false)
	if err != nil {
		return false
	}
	parent, repo, _ := strings.Cut(candidate.Identity, "/")
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || name == "" || strings.Contains(name, "/") {
		return false
	}
	for _, entry := range policy.Owners {
		if ownerIdentity(entry) == parent+"/"+owner {
			return true
		}
	}
	return false
}

func admitRepo(policy Admission, repo, label, host string) error {
	if policy.admitAll() {
		return nil
	}
	refusal := fmt.Errorf("repo %q is not admitted on host %q", label, host)
	if len(policy.Repos) == 0 && len(policy.Owners) == 0 {
		return refusal
	}
	origin, err := command("", "git", "-C", repo, "remote", "get-url", "origin")
	if err != nil {
		return refusal
	}
	identity := repoIdentity(origin)
	for _, allowed := range policy.Repos {
		if strings.TrimSpace(allowed) != "" && identity == repoIdentity(allowed) {
			return nil
		}
	}
	if policy.admitsOwner(origin) {
		return nil
	}
	return refusal
}

// Policy is owned by this process's target, independent of all request fields.
func admitTargetRepo(repo, label, host string) error {
	cfg, err := targetPolicy()
	if err != nil {
		return fmt.Errorf("target admission config: %w", err)
	}
	return admitRepo(cfg.Admission, repo, label, host)
}

// targetPolicy never accepts a sender-selected path or host alias.
func targetPolicy() (Config, error) {
	cfg, err := loadTarget("", "")
	if os.IsNotExist(err) && os.Getenv("HEV_DISPATCH_CONFIG") == "" {
		return cfg, nil
	}
	return cfg, err
}
