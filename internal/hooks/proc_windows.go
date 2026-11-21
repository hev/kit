//go:build windows

package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// ownGroup keeps the default cancellation (kill the handler process) on
// Windows.
func ownGroup(c *exec.Cmd) {}

func handlerCommand(ctx context.Context, exe, event string) (*exec.Cmd, error) {
	switch strings.ToLower(filepath.Ext(exe)) {
	case ".cmd", ".bat":
		// cmd.exe parses its command line again. Refuse expansion and
		// metacharacters before handing a literal path to it.
		if strings.ContainsAny(exe+event, "\"^&|<>()%!\r\n") {
			return nil, fmt.Errorf("unsafe batch handler path or event")
		}
		c := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
		c.SysProcAttr = &syscall.SysProcAttr{
			CmdLine: `cmd.exe /d /s /c ""` + exe + `" "` + event + `""`,
		}
		return c, nil
	default:
		return exec.CommandContext(ctx, exe, event), nil
	}
}
