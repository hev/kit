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
	Repos []string `toml:"repos"`
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

func admitRepo(policy Admission, repo, label, host string) error {
	if policy.Repos == nil {
		return nil
	}
	refusal := fmt.Errorf("repo %q is not admitted on host %q", label, host)
	if len(policy.Repos) == 0 {
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
