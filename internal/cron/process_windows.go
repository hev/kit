package cron

import (
	"os/exec"

	"github.com/hev/kit/internal/processjob"
)

func startTree(cmd *exec.Cmd) (func(), error) {
	return processjob.Start(cmd)
}
