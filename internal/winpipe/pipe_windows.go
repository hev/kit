//go:build windows

// Package winpipe is the private local message-pipe backend for nativehost.
// It grants no execution authority and never impersonates beyond identification.
package winpipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	"golang.org/x/sys/windows"
)

const MaxFrame = 64 << 10

var ErrHeld = errors.New("Windows owner pipe held")
var impersonate = windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")
var peekPipe = windows.NewLazySystemDLL("kernel32.dll").NewProc("PeekNamedPipe")

func currentSID() (string, error) {
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", e
	}
	return u.User.Sid.String(), nil
}
func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func Name(configPath string) (string, error) {
	sid, e := currentSID()
	if e != nil {
		return "", e
	}
	if fs.PathSyntax(configPath) != nil {
		return "", ErrHeld
	}
	return `\\.\pipe\hev-nativehost-v1-` + sum(sid) + "-" + sum(strings.ToLower(configPath)), nil
}
func Validate(path string) error {
	sid, e := currentSID()
	if e != nil {
		return e
	}
	prefix := `\\.\pipe\hev-nativehost-v1-` + sum(sid) + "-"
	if !strings.HasPrefix(path, prefix) || len(path) != len(prefix)+64 {
		return ErrHeld
	}
	tail := path[len(prefix):]
	if strings.ToLower(tail) != tail {
		return ErrHeld
	}
	_, e = hex.DecodeString(tail)
	return e
}

type address string

func (a address) Network() string { return "windows-owner-pipe" }
func (a address) String() string  { return string(a) }

type Listener struct {
	mu                sync.Mutex
	path              string
	pending           windows.Handle
	accepting, closed bool
}

func newInstance(path string, first bool) (windows.Handle, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return 0, e
	}
	sd, e := fs.PrivateDescriptor()
	if e != nil {
		return 0, e
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	flags := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	h, e := windows.CreateNamedPipe(p, flags, windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_REJECT_REMOTE_CLIENTS, 32, MaxFrame+1, MaxFrame+1, 0, &sa)
	runtime.KeepAlive(sd)
	if e != nil {
		return 0, e
	}
	if e = fs.VerifyPrivateHandle(h, true, false); e != nil {
		return 0, errors.Join(e, windows.CloseHandle(h))
	}
	return h, nil
}
func Listen(path string) (*Listener, error) {
	if e := Validate(path); e != nil {
		return nil, e
	}
	h, e := newInstance(path, true)
	if e != nil {
		return nil, e
	}
	return &Listener{path: path, pending: h}, nil
}
func (l *Listener) Addr() net.Addr { return address(l.path) }
func (l *Listener) Attested() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closed && l.pending != 0 && Validate(l.path) == nil
}
func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.accepting {
		e := windows.CancelIoEx(l.pending, nil)
		if errors.Is(e, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return e
	}
	e := windows.CloseHandle(l.pending)
	l.pending = 0
	return e
}
func (l *Listener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if l.closed || l.accepting {
		l.mu.Unlock()
		return nil, net.ErrClosed
	}
	l.accepting = true
	h := l.pending
	l.mu.Unlock()
	event, e := windows.CreateEvent(nil, 1, 0, nil)
	if e != nil {
		l.mu.Lock()
		l.accepting = false
		if l.closed {
			l.pending = 0
			e = errors.Join(e, windows.CloseHandle(h))
		}
		l.mu.Unlock()
		return nil, e
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	l.mu.Lock()
	if l.closed {
		l.accepting = false
		l.pending = 0
		l.mu.Unlock()
		return nil, errors.Join(net.ErrClosed, windows.CloseHandle(h))
	}
	e = windows.ConnectNamedPipe(h, &ov)
	l.mu.Unlock()
	if errors.Is(e, windows.ERROR_IO_PENDING) {
		var done uint32
		e = windows.GetOverlappedResult(h, &ov, &done, true)
	}
	if errors.Is(e, windows.ERROR_PIPE_CONNECTED) {
		e = nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.accepting = false
	if l.closed {
		l.pending = 0
		return nil, errors.Join(net.ErrClosed, windows.CloseHandle(h))
	}
	if e != nil {
		return nil, e
	}
	next, e := newInstance(l.path, false)
	if e != nil {
		l.closed = true
		l.pending = 0
		return nil, errors.Join(e, windows.CloseHandle(h))
	}
	l.pending = next
	return &Conn{h: h, path: l.path, server: true}, nil
}

func Dial(ctx context.Context, path string) (net.Conn, error) {
	if e := Validate(path); e != nil {
		return nil, e
	}
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	var h windows.Handle
	for {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		h, e = windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if e == nil {
			break
		}
		if !errors.Is(e, windows.ERROR_PIPE_BUSY) {
			return nil, e
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	fail := func(e error) (net.Conn, error) { return nil, errors.Join(e, windows.CloseHandle(h)) }
	if e = fs.VerifyPrivateHandle(h, true, false); e != nil {
		return fail(e)
	}
	mode := uint32(windows.PIPE_READMODE_MESSAGE)
	if e = windows.SetNamedPipeHandleState(h, &mode, nil, nil); e != nil {
		return fail(e)
	}
	c := &Conn{h: h, path: path}
	if deadline, ok := ctx.Deadline(); ok {
		c.readDeadline = deadline
		c.writeDeadline = deadline
	}
	return c, nil
}

type Conn struct {
	mu                            sync.Mutex
	readMu, writeMu               sync.Mutex
	ops                           sync.WaitGroup
	closeOnce                     sync.Once
	h                             windows.Handle
	path                          string
	server, closed                bool
	closeErr                      error
	readDeadline, writeDeadline   time.Time
	prefix                        []byte
	readEnd, wrote, authenticated bool
}

func (c *Conn) LocalAddr() net.Addr  { return address(c.path) }
func (c *Conn) RemoteAddr() net.Addr { return address(c.path) }
func (c *Conn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	c.readDeadline = t
	return nil
}
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	c.writeDeadline = t
	return nil
}

// io performs one overlapped operation, retaining its buffer/OVERLAPPED until
// cancellation is acknowledged. No stack OVERLAPPED escapes a completed call.
func (c *Conn) io(buf []byte, write bool, override time.Time) (int, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, net.ErrClosed
	}
	h := c.h
	deadline := c.readDeadline
	if write {
		deadline = c.writeDeadline
	}
	if !override.IsZero() {
		deadline = override
	}
	c.ops.Add(1)
	c.mu.Unlock()
	defer c.ops.Done()
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return 0, os.ErrDeadlineExceeded
	}
	event, e := windows.CreateEvent(nil, 1, 0, nil)
	if e != nil {
		return 0, e
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	var count uint32
	if write {
		e = windows.WriteFile(h, buf, &count, &ov)
	} else {
		e = windows.ReadFile(h, buf, &count, &ov)
	}
	if e == nil || errors.Is(e, windows.ERROR_MORE_DATA) {
		e = windows.GetOverlappedResult(h, &ov, &count, false)
		if errors.Is(e, windows.ERROR_IO_INCOMPLETE) {
			e = windows.ERROR_IO_PENDING
		}
	}
	if errors.Is(e, windows.ERROR_IO_PENDING) {
		for {
			c.mu.Lock()
			closed := c.closed
			deadline = c.readDeadline
			if write {
				deadline = c.writeDeadline
			}
			c.mu.Unlock()
			if !override.IsZero() {
				deadline = override
			}
			expired := !deadline.IsZero() && !time.Now().Before(deadline)
			if closed || expired {
				cancel := windows.CancelIoEx(h, &ov)
				if errors.Is(cancel, windows.ERROR_NOT_FOUND) {
					cancel = nil
				}
				finish := windows.GetOverlappedResult(h, &ov, &count, true)
				if errors.Is(finish, windows.ERROR_OPERATION_ABORTED) {
					finish = nil
				}
				cause := error(os.ErrDeadlineExceeded)
				if closed {
					cause = net.ErrClosed
				}
				return int(count), errors.Join(cause, cancel, finish)
			}
			// Re-read deadlines at a bounded interval so SetDeadline affects an
			// already pending operation without issuing another read/write request.
			timeout := uint32(25)
			if !deadline.IsZero() {
				remaining := time.Until(deadline)
				ms := (remaining + time.Millisecond - 1) / time.Millisecond
				if ms < 25 {
					timeout = uint32(ms)
				}
			}
			status, we := windows.WaitForSingleObject(event, timeout)
			if we != nil {
				cancel := windows.CancelIoEx(h, &ov)
				if errors.Is(cancel, windows.ERROR_NOT_FOUND) {
					cancel = nil
				}
				finish := windows.GetOverlappedResult(h, &ov, &count, true)
				return int(count), errors.Join(we, cancel, finish)
			}
			if status == uint32(windows.WAIT_TIMEOUT) {
				continue
			}
			if status != windows.WAIT_OBJECT_0 {
				cancel := windows.CancelIoEx(h, &ov)
				if errors.Is(cancel, windows.ERROR_NOT_FOUND) {
					cancel = nil
				}
				finish := windows.GetOverlappedResult(h, &ov, &count, true)
				return int(count), errors.Join(ErrHeld, cancel, finish)
			}
			e = windows.GetOverlappedResult(h, &ov, &count, false)
			break
		}
	}
	runtime.KeepAlive(buf)
	if errors.Is(e, windows.ERROR_BROKEN_PIPE) || errors.Is(e, windows.ERROR_PIPE_NOT_CONNECTED) {
		return int(count), io.EOF
	}
	return int(count), e
}
func (c *Conn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	if c.readEnd {
		return 0, io.EOF
	}
	n, e := c.io(p, false, time.Time{})
	if errors.Is(e, windows.ERROR_MORE_DATA) {
		return n, nil
	}
	if e == nil {
		c.readEnd = true
		if n == 0 {
			return 0, io.EOF
		}
	}
	return n, e
}
func (c *Conn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if len(p) == 0 || len(p) > MaxFrame || c.wrote {
		return 0, ErrHeld
	}
	n, e := c.io(p, true, time.Time{})
	if e == nil && n != len(p) {
		e = io.ErrShortWrite
	}
	if e == nil {
		c.wrote = true
	}
	return n, e
}
func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		// Preserve a buffered response until the client has consumed and closed its
		// one-message connection. This bounded read is NOT FlushFileBuffers, whose
		// synchronous pipe semantics could hang forever on an untrusted reader.
		c.writeMu.Lock()
		wrote := c.wrote
		c.writeMu.Unlock()
		if c.server && wrote {
			var b [1]byte
			_, _ = c.io(b[:], false, time.Now().Add(time.Second))
		}
		c.mu.Lock()
		c.closed = true
		h := c.h
		e := windows.CancelIoEx(h, nil)
		c.mu.Unlock()
		if errors.Is(e, windows.ERROR_NOT_FOUND) {
			e = nil
		}
		c.ops.Wait()
		c.closeErr = errors.Join(e, windows.CloseHandle(h))
	})
	return c.closeErr
}

func peerSID(pid uint32) (string, error) {
	if pid == 0 {
		return "", ErrHeld
	}
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if e != nil {
		return "", e
	}
	defer windows.CloseHandle(h)
	var token windows.Token
	if e = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); e != nil {
		return "", e
	}
	defer token.Close()
	u, e := token.GetTokenUser()
	if e != nil {
		return "", e
	}
	return u.User.Sid.String(), nil
}

// Authenticate proves both ends from kernel identity and the actual pipe DACL.
// Server impersonation is identification-only and always reverted on this exact
// locked OS thread before it can return to the Go scheduler.
func (c *Conn) Authenticate() error {
	own, e := currentSID()
	if e != nil {
		return e
	}
	if e = fs.VerifyPrivateHandle(c.h, true, false); e != nil {
		return e
	}
	var pid uint32
	if !c.server {
		e = windows.GetNamedPipeServerProcessId(c.h, &pid)
		if e != nil {
			return e
		}
		sid, e := peerSID(pid)
		if e != nil || sid != own {
			return errors.Join(ErrHeld, e)
		}
		c.mu.Lock()
		c.authenticated = true
		c.mu.Unlock()
		return nil
	}
	if e = windows.GetNamedPipeClientProcessId(c.h, &pid); e != nil {
		return e
	}
	sid, e := peerSID(pid)
	if e != nil || sid != own {
		return errors.Join(ErrHeld, e)
	}
	// Impersonation identity refers to the last message read; retain its first
	// byte for readRequest so authentication precedes payload parsing/dispatch.
	c.readMu.Lock()
	var b [1]byte
	n, e := c.io(b[:], false, time.Time{})
	if errors.Is(e, windows.ERROR_MORE_DATA) {
		e = nil
	} else if e == nil {
		c.readEnd = true
	}
	if e == nil && n == 1 {
		c.prefix = append(c.prefix, b[0])
	}
	c.readMu.Unlock()
	if e != nil || n != 1 {
		return errors.Join(ErrHeld, e)
	}
	runtime.LockOSThread()
	reverted := false
	defer func() {
		if reverted {
			runtime.UnlockOSThread()
		}
	}()
	ok, _, callErr := impersonate.Call(uintptr(c.h))
	if ok == 0 {
		reverted = true
		return errors.Join(ErrHeld, callErr)
	}
	var token windows.Token
	thread, _ := windows.GetCurrentThread()
	tokenErr := windows.OpenThreadToken(thread, windows.TOKEN_QUERY, true, &token)
	var tokenSID string
	if tokenErr == nil {
		u, ue := token.GetTokenUser()
		tokenErr = ue
		if ue == nil {
			tokenSID = u.User.Sid.String()
		}
		tokenErr = errors.Join(tokenErr, token.Close())
	}
	revertErr := windows.RevertToSelf()
	reverted = revertErr == nil
	if tokenErr != nil || revertErr != nil || tokenSID != own {
		return errors.Join(ErrHeld, tokenErr, revertErr)
	}
	c.mu.Lock()
	c.authenticated = true
	c.mu.Unlock()
	return nil
}
func ReadRequest(conn net.Conn) ([]byte, error) {
	c, ok := conn.(*Conn)
	if !ok || !c.server {
		return nil, ErrHeld
	}
	c.mu.Lock()
	authenticated := c.authenticated
	c.mu.Unlock()
	if !authenticated {
		return nil, ErrHeld
	}
	data, e := io.ReadAll(io.LimitReader(c, MaxFrame+1))
	if e != nil || len(data) == 0 || len(data) > MaxFrame {
		return nil, errors.Join(ErrHeld, e)
	}
	// Reject already queued trailing messages; never process another payload on
	// this connection. Later writes are discarded when the owned connection closes.
	var available uint32
	okValue, _, pe := peekPipe.Call(uintptr(c.h), 0, 0, 0, uintptr(unsafe.Pointer(&available)), 0)
	if okValue == 0 {
		return nil, errors.Join(ErrHeld, pe)
	}
	if available != 0 {
		return nil, ErrHeld
	}
	return data, nil
}
func WriteRequest(conn net.Conn, data []byte) error {
	c, ok := conn.(*Conn)
	if !ok || c.server || len(data) == 0 || len(data) > MaxFrame {
		return ErrHeld
	}
	c.mu.Lock()
	authenticated := c.authenticated
	c.mu.Unlock()
	if !authenticated {
		return ErrHeld
	}
	_, e := c.Write(data)
	return e
}
