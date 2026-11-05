package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

func notificationEnvironment() []string {
	env := []string{}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(key, "PEERS_FROM") {
			env = append(env, value)
		}
	}
	return withoutPluginOnlyEnv(env)
}

var notificationExecutable = os.Executable

// hostName is replaced in tests.
var hostName = os.Hostname

// notificationHost is the peers host label for code-authored senders.
func notificationHost() string {
	host := os.Getenv("PEERS_SELF")
	if host == "" {
		host, _ = hostName()
		host = strings.ToLower(strings.SplitN(host, ".", 2)[0])
		if strings.Contains(host, "mac-mini") {
			host = "mini"
		} else if strings.Contains(host, "macbook") {
			host = "macbook"
		}
	}
	return host
}

// Detached validation RPCs have no pane ancestry. Select the run explicitly.
// The external CLI requires a live session and accepts its local session name;
// the native engine also supports a qualified sender after the session exits.
func notifyValidationResult(ctx context.Context, args []string, m Meta, r Result) error {
	if len(args) == 0 {
		return nil
	}
	for _, arg := range args {
		if m.Reply == "" && strings.Contains(arg, "{reply}") {
			return nil
		}
	}
	external := strings.TrimSuffix(filepath.Base(args[0]), ".exe") == "peers"
	native := len(args) > 1 && strings.TrimSuffix(filepath.Base(args[0]), ".exe") == "hev" && args[1] == "peers"
	if !external && !native {
		return Notify(ctx, args, m, r)
	}
	sender := notificationHost() + "/d-" + m.Name
	live := notificationSessionExists(m.Name)
	if external && live {
		sender = "d-" + m.Name
	}
	if !live {
		exe, err := notificationExecutable()
		if err != nil {
			return err
		}
		tail := args[1:]
		if native {
			tail = args[2:]
		}
		args = append([]string{exe, "peers"}, tail...)
	}
	// Pass sender as a flag so Notify can always discard inherited PEERS_FROM.
	args = append(append([]string{}, args...), "--from", sender)
	// Bypass any configured external backend when the session no longer exists.
	// This is chosen before delivery; never retry an uncertain send.
	if !live || native {
		return notify(ctx, args, m, r, true)
	}
	return Notify(ctx, args, m, r)
}
