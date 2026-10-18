//go:build windows

package winpipe

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func proofListener(t *testing.T) (*Listener, string) {
	t.Helper()
	name, e := Name(filepath.Join(t.TempDir(), "host.json"))
	if e != nil {
		t.Fatal(e)
	}
	l, e := Listen(name)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	return l, name
}
func TestWindowsOwnerPipeMessageRoundTrip(t *testing.T) {
	l, name := proofListener(t)
	done := make(chan error, 1)
	go func() {
		c, e := l.Accept()
		if e != nil {
			done <- e
			return
		}
		defer c.Close()
		if e = c.SetDeadline(time.Now().Add(5 * time.Second)); e != nil {
			done <- e
			return
		}
		p := c.(*Conn)
		if e = p.Authenticate(); e != nil {
			done <- e
			return
		}
		b, e := ReadRequest(c)
		if e != nil {
			done <- e
			return
		}
		if string(b) != "{\"version\":\"proof\"}\n" {
			done <- ErrHeld
			return
		}
		_, e = c.Write([]byte("accepted\n"))
		done <- e
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, e := Dial(ctx, name)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.(*Conn).Authenticate(); e != nil {
		t.Fatal("server SID/DACL", e)
	}
	if e = WriteRequest(c, []byte("{\"version\":\"proof\"}\n")); e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(io.LimitReader(c, 4097))
	if e != nil || string(b) != "accepted\n" {
		t.Fatal(string(b), e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal("client SID/frame", e)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func TestWindowsOwnerPipeExclusiveNamespace(t *testing.T) {
	l, name := proofListener(t)
	if !l.Attested() {
		t.Fatal("own listener not attested")
	}
	other, e := Listen(name)
	if e == nil {
		other.Close()
		t.Fatal("second owner claimed same endpoint")
	}
	if e = Validate(`\\server\pipe\remote`); e == nil {
		t.Fatal("remote pipe accepted")
	}
	wrong := name[:len(name)-1] + "Z"
	if e = Validate(wrong); e == nil {
		t.Fatal("invalid namespace accepted")
	}
}
func TestWindowsOwnerPipeCloseUnblocksAccept(t *testing.T) {
	for i := 0; i < 10; i++ {
		l, _ := proofListener(t)
		done := make(chan error, 1)
		go func() {
			c, e := l.Accept()
			if c != nil {
				c.Close()
			}
			done <- e
		}()
		if e := l.Close(); e != nil {
			t.Fatal(e)
		}
		select {
		case e := <-done:
			if !errors.Is(e, net.ErrClosed) {
				t.Fatal("close accept", e)
			}
		case <-time.After(time.Second):
			t.Fatal("Accept survived Close")
		}
	}
}
func TestWindowsOwnerPipeDeadlineChangeAndCancel(t *testing.T) {
	l, name := proofListener(t)
	accepted := make(chan net.Conn, 1)
	go func() {
		c, e := l.Accept()
		if e == nil {
			accepted <- c
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, e := Dial(ctx, name)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	var server net.Conn
	select {
	case server = <-accepted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	defer server.Close()
	if e = server.SetReadDeadline(time.Now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { var b [1]byte; _, e := server.Read(b[:]); done <- e }()
	time.Sleep(20 * time.Millisecond)
	if e = server.SetReadDeadline(time.Now()); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if !errors.Is(e, os.ErrDeadlineExceeded) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("updated deadline did not cancel pending read")
	}
}
func TestWindowsOwnerPipeRejectsOversizeAndTrailingFrames(t *testing.T) {
	for _, mode := range []string{"oversize", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			l, name := proofListener(t)
			accepted := make(chan *Conn, 1)
			go func() {
				c, e := l.Accept()
				if e == nil {
					accepted <- c.(*Conn)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			connection, e := Dial(ctx, name)
			if e != nil {
				t.Fatal(e)
			}
			client := connection.(*Conn)
			defer client.Close()
			var server *Conn
			select {
			case server = <-accepted:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			defer server.Close()
			if mode == "oversize" {
				if e = WriteRequest(client, bytes.Repeat([]byte{'x'}, MaxFrame+1)); e == nil {
					t.Fatal("oversize write accepted")
				}
				return
			}
			// Deliberately bypass the SDK one-write guard to act as a hostile local
			// client. Both messages are queued before the authenticated request read.
			if _, e = client.io([]byte("first"), true, time.Time{}); e != nil {
				t.Fatal(e)
			}
			if _, e = client.io([]byte("second"), true, time.Time{}); e != nil {
				t.Fatal(e)
			}
			if e = server.SetDeadline(time.Now().Add(time.Second)); e != nil {
				t.Fatal(e)
			}
			if e = server.Authenticate(); e != nil {
				t.Fatal(e)
			}
			if _, e = ReadRequest(server); e == nil {
				t.Fatal("queued trailing message accepted")
			}
		})
	}
}
