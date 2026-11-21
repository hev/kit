package dispatch

import (
	"context"
	"fmt"
	"github.com/hev/kit/internal/hooks"
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

// targetPolicy never accepts a sender-selected path or host alias.
func targetPolicy() (Config, error) {
	cfg, err := loadTarget("", "")
	if os.IsNotExist(err) && os.Getenv("HEV_DISPATCH_CONFIG") == "" {
		return cfg, nil
	}
	return cfg, err
}

// admitWork applies local repository policy before letting hooks add restrictions.
func admitWork(cfg Config, repo string, m Meta, action string) error {
	data := map[string]any{"name": m.Name, "repo": m.Repo, "action": action}
	ctx := context.Background()
	if err := admitRepo(cfg.Admission, repo, m.Repo, m.Host); err != nil {
		hooks.Meta(ctx, "admission-denied", data, err.Error())
		return err
	}
	result := hooks.Meta(ctx, "admission-check", data, "")
	if result.Decision == hooks.Block || result.Decision == hooks.Halt {
		reason := result.Reason
		if reason == "" {
			reason = "denied by an admission hook"
		}
		hooks.Meta(ctx, "admission-denied", data, reason)
		return fmt.Errorf("admission denied: %s", reason)
	}
	return nil
}

func orderEvent(event string, m Meta, action string) {
	data := map[string]any{"name": m.Name, "repo": m.Repo, "worker_harness": m.Harness}
	if action != "" {
		data["action"] = action
	}
	hooks.Meta(context.Background(), event, data, "")
}

// admitTargetRepo uses the receiver-owned policy regardless of sender fields.
func admitTargetRepo(repo string, m Meta, action string) error {
	cfg, err := targetPolicy()
	if err != nil {
		return fmt.Errorf("target admission config: %w", err)
	}
	return admitWork(cfg, repo, m, action)
}
