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

	"github.com/hev/kit/internal/processjob"
)

func runHandler(ctx context.Context, c *exec.Cmd) error {
	// Use job cancellation so children and inherited pipes are covered.
	c.Cancel = nil
	stop, err := processjob.Start(c)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		stop()
		return err
	case <-ctx.Done():
		stop()
		<-done
		return ctx.Err()
	}
}

func handlerCommand(ctx context.Context, exe, event string) (*exec.Cmd, error) {
	switch strings.ToLower(filepath.Ext(exe)) {
	case ".cmd", ".bat":
		// cmd.exe parses its command line again. Refuse expansion and
		// metacharacters before handing a literal path to it.
		if strings.ContainsAny(exe, "\"%!\r\n") || strings.ContainsAny(event, "\"^&|<>()%!\r\n") {
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
