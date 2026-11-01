package dispatch

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/hev/kit/internal/trace"
)

// Admission leaves room for other resource kinds without mixing their identities.
type Admission struct {
	Repos []string `toml:"repos"`
}

// repoIdentity compares origin locations, never the sender's repo label.
// Transport and SSH user do not identify a repository. Repository path case does.
func repoIdentity(remote string) string {
	remote = strings.TrimSpace(remote)
	if trace.WindowsPath(remote) || strings.HasPrefix(remote, "//") {
		remote = strings.ReplaceAll(remote, `\`, "/")
		if len(remote) > 1 && remote[1] == ':' {
			remote = strings.ToLower(remote[:1]) + remote[1:]
		}
		return strings.TrimSuffix(path.Clean(remote), ".git")
	}
	if !strings.Contains(remote, "://") && !strings.HasPrefix(remote, "/") {
		if colon := strings.IndexByte(remote, ':'); colon >= 0 {
			remote = "ssh://" + remote[:colon] + "/" + remote[colon+1:]
		}
	}
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		return strings.ToLower(u.Host) + "/" + strings.TrimSuffix(strings.Trim(path.Clean(u.Path), "/"), ".git")
	}
	return strings.TrimSuffix(path.Clean(remote), ".git")
}

func admitRepo(h Host, repo, label, host string) error {
	if h.Admission.Repos == nil {
		return nil
	}
	refusal := fmt.Errorf("repo %q is not admitted on host %q", label, host)
	if len(h.Admission.Repos) == 0 {
		return refusal
	}
	origin, err := command("", "git", "-C", repo, "remote", "get-url", "origin")
	if err != nil {
		return refusal
	}
	identity := repoIdentity(origin)
	for _, allowed := range h.Admission.Repos {
		if strings.TrimSpace(allowed) != "" && identity == repoIdentity(allowed) {
			return nil
		}
	}
	return refusal
}
