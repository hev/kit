package cron

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hev/kit/internal/redact"
)

type capture struct {
	mu       sync.Mutex
	streams  [2]bytes.Buffer
	size     int
	overflow bool
}

type captureStream struct {
	capture *capture
	index   int
}

func (s captureStream) Write(p []byte) (int, error) {
	c := s.capture
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(p) > outputLimit-c.size {
		c.overflow = true
	}
	if !c.overflow {
		_, _ = c.streams[s.index].Write(p)
		c.size += len(p)
	}
	return len(p), nil
}

var outputScrubber = sync.OnceValues(func() (*redact.Scrubber, error) {
	var salt [32]byte
	_, _ = rand.Read(salt[:])
	return redact.New(salt[:])
})

func (c *capture) scrub(values map[string]string, paths ...string) (string, bool) {
	if c.overflow {
		return "", true
	}
	scrubber, err := outputScrubber()
	if err != nil {
		return "", true
	}
	var terms []string
	for _, v := range values {
		if v != "" {
			terms = append(terms, v)
		}
	}
	for _, path := range paths {
		if filepath.IsAbs(path) && filepath.Dir(path) != path {
			terms = append(terms, path)
			if slash := filepath.ToSlash(path); slash != path {
				terms = append(terms, slash)
			}
		}
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	pairs := make([]string, 0, len(terms)*2)
	for _, term := range terms {
		pairs = append(pairs, term, "[REDACTED]")
	}
	replacer := strings.NewReplacer(pairs...)
	var clean string
	// Keep streams separate through redaction so interleaving cannot break a
	// secret match. Present stdout first, then stderr.
	for i := range c.streams {
		text, _ := scrubber.Text(replacer.Replace(c.streams[i].String()))
		if text != "" && clean != "" && !strings.HasSuffix(clean, "\n") {
			clean += "\n"
		}
		clean += strings.ToValidUTF8(text, "\uFFFD")
	}
	if len(clean) > outputLimit {
		return "", true
	}
	return clean, false
}

func execute(ctx context.Context, j Job, home string, values map[string]string, r *Run) {
	cmd := exec.CommandContext(ctx, j.Cmd[0], j.Cmd[1:]...)
	cmd.Dir = j.Workdir
	if cmd.Dir == "" || cmd.Dir == "~" {
		cmd.Dir = home
	} else if strings.HasPrefix(cmd.Dir, "~/") || strings.HasPrefix(cmd.Dir, `~\`) {
		cmd.Dir = filepath.Join(home, cmd.Dir[2:])
	} else if !filepath.IsAbs(cmd.Dir) {
		cmd.Dir = filepath.Join(home, cmd.Dir)
	}
	cmd.Env = cmd.Environ()
	for name, value := range values {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	output := &capture{}
	cmd.Stdout = captureStream{capture: output, index: 0}
	cmd.Stderr = captureStream{capture: output, index: 1}
	cmd.WaitDelay = 250 * time.Millisecond
	// The supervisor cancels the entire process tree when the deadline fires.
	cmd.Cancel = nil
	stop, err := startTree(cmd)
	if err == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
			stop()
		case <-ctx.Done():
			stop()
			err = <-done
		}
	}
	if cmd.ProcessState != nil {
		exit := cmd.ProcessState.ExitCode()
		r.ExitCode = &exit
	}
	paths := append([]string{home, cmd.Dir, cmd.Path}, j.Cmd[1:]...)
	var omitted bool
	r.Output, omitted = output.scrub(values, paths...)
	switch {
	case ctx.Err() != nil:
		setContextResult(ctx, r)
	case err != nil:
		r.Status = "failed"
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			r.Message = "could not start or collect job process"
		}
	default:
		r.Status = "ok"
	}
	if omitted {
		if r.Message != "" {
			r.Message += ". "
		}
		r.Message += "job output omitted"
	}
}

func setContextResult(ctx context.Context, r *Run) {
	r.Status = "canceled"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		r.Status = "timeout"
	}
}
