// Package exitcode is hev's exit-status convention. Commands return errors
// tagged with a code; the root maps an error to the status the process exits
// with. Scripts may rely on these numbers.
package exitcode

import (
	"context"
	"errors"
	"fmt"
)

const (
	OK          = 0   // success
	Error       = 1   // runtime error
	Usage       = 2   // invalid usage: unknown command or flag, missing or extra arguments
	Auth        = 3   // authentication or permission refused
	NotFound    = 4   // the named thing does not exist
	Conflict    = 5   // the request conflicts with current state
	Interrupted = 130 // stopped by Ctrl-C (SIGINT)
)

// Doc is the convention as shown in help and docs.
const Doc = `Exit status:
  0    success
  1    runtime error
  2    invalid usage (unknown command or flag, missing or extra arguments)
  3    authentication or permission refused
  4    not found
  5    conflict with current state
  130  interrupted (Ctrl-C)
Some commands document their own codes (hev secrets, hev intake, hev peers).`

// Coder is an error that carries its exit status.
type Coder interface{ ExitCode() int }

type coded struct {
	code int
	err  error
}

func (e *coded) Error() string { return e.err.Error() }
func (e *coded) Unwrap() error { return e.err }
func (e *coded) ExitCode() int { return e.code }

// With tags err with code. A nil err stays nil.
func With(code int, err error) error {
	if err == nil {
		return nil
	}
	return &coded{code: code, err: err}
}

func UsageErr(err error) error    { return With(Usage, err) }
func NotFoundErr(err error) error { return With(NotFound, err) }

func Usagef(format string, a ...any) error    { return With(Usage, fmt.Errorf(format, a...)) }
func NotFoundf(format string, a ...any) error { return With(NotFound, fmt.Errorf(format, a...)) }
func Authf(format string, a ...any) error     { return With(Auth, fmt.Errorf(format, a...)) }
func Conflictf(format string, a ...any) error { return With(Conflict, fmt.Errorf(format, a...)) }

type hinted struct {
	err  error
	hint string
}

func (e *hinted) Error() string { return e.err.Error() }
func (e *hinted) Unwrap() error { return e.err }

// WithHint attaches a next step, such as the command that fixes the error.
// The root prints it on its own line after the error.
func WithHint(err error, hint string) error {
	if err == nil || hint == "" {
		return err
	}
	return &hinted{err: err, hint: hint}
}

// Hint is the outermost hint attached to err, or "".
func Hint(err error) string {
	var h *hinted
	if errors.As(err, &h) {
		return h.hint
	}
	return ""
}

// FromHTTP maps an HTTP error status to the convention: 401 and 403 are Auth,
// 404 and 410 NotFound, 409 and 412 Conflict, other 4xx Error, as is 5xx.
func FromHTTP(status int, err error) error {
	switch status {
	case 401, 403:
		return With(Auth, err)
	case 404, 410:
		return With(NotFound, err)
	case 409, 412:
		return With(Conflict, err)
	}
	return err
}

// Of is the exit status for err: 0 for nil, the tagged code when err carries
// one, Interrupted for a cancelled context, otherwise Error.
func Of(err error) int {
	if err == nil {
		return OK
	}
	var c Coder
	if errors.As(err, &c) {
		return c.ExitCode()
	}
	if errors.Is(err, context.Canceled) {
		return Interrupted
	}
	return Error
}
