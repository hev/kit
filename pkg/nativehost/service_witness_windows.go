//go:build windows

package nativehost

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/windows"
)

type windowsServiceWitness struct {
	mu           sync.Mutex
	job, process windows.Handle
	birth        n.Birth
	closeErr     error
}

func platformOpenServiceWitness(birth n.Birth, path string) (_ ServiceWitness, err error) {
	// Authenticate all original SID/image/session/birth/config fields while the
	// host is live. The subsequent retained handles must identify that same birth.
	if _, e := platformObserveService(birth, path); e != nil {
		return nil, e
	}
	name, e := windowsServiceName(path, birth)
	if e != nil {
		return nil, e
	}
	ptr, e := windows.UTF16PtrFromString(name)
	if e != nil {
		return nil, e
	}
	raw, _, e := serviceOpenJob.Call(uintptr(0x0004|0x00020000), 0, uintptr(unsafe.Pointer(ptr)))
	if raw == 0 {
		return nil, e
	}
	w := &windowsServiceWitness{job: windows.Handle(raw), birth: birth}
	fail := func(e error) (ServiceWitness, error) { return nil, errors.Join(e, w.Close()) }
	if e = fs.VerifyPrivateJobHandle(w.job); e != nil {
		return fail(e)
	}
	if e = windowsServiceLimits(w.job); e != nil {
		return fail(e)
	}
	w.process, e = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(birth.PID))
	if e != nil {
		return fail(e)
	}
	var creation, exit, kernel, user windows.Filetime
	if e = windows.GetProcessTimes(w.process, &creation, &exit, &kernel, &user); e != nil {
		return fail(e)
	}
	if strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10) != birth.Start {
		return fail(ErrHeld)
	}
	member, e := windowsServiceContains(w.job, w.process)
	if e != nil || !member {
		return fail(errors.Join(ErrHeld, e))
	}
	actual, e := n.ProcessBirth(birth.PID)
	if e != nil || actual != birth {
		return fail(errors.Join(ErrHeld, e))
	}
	state, e := windows.WaitForSingleObject(w.process, 0)
	if e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return fail(errors.Join(ErrHeld, e))
	}
	return w, nil
}
func (w *windowsServiceWitness) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.process != 0 {
		w.closeErr = errors.Join(w.closeErr, windows.CloseHandle(w.process))
		w.process = 0
	}
	if w.job != 0 {
		w.closeErr = errors.Join(w.closeErr, windows.CloseHandle(w.job))
		w.job = 0
	}
	return w.closeErr
}
func (w *windowsServiceWitness) Observe() (out ServiceObservation, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.job == 0 || w.process == 0 {
		return out, ErrHeld
	}
	if e := fs.VerifyPrivateJobHandle(w.job); e != nil {
		return out, e
	}
	if e := windowsServiceLimits(w.job); e != nil {
		return out, e
	}
	before, e := windowsServicePIDsAllowEmpty(w.job, true)
	if e != nil {
		return out, e
	}
	members := make([]n.Birth, 0, len(before))
	var retained []windows.Handle
	defer func() {
		for _, handle := range retained {
			err = errors.Join(err, windows.CloseHandle(handle))
		}
		if err != nil {
			out = ServiceObservation{}
		}
	}()
	// Hold each queried member while authenticating its original creation time;
	// no handle is used to signal, adopt or reacquire the historical host.
	for _, pid := range before {
		process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
		if e != nil {
			return out, e
		}
		retained = append(retained, process)
		b, e := n.ProcessBirth(int(pid))
		var creation, exit, kernel, user windows.Filetime
		te := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user)
		member, me := windowsServiceContains(w.job, process)
		var ce error
		if e != nil || te != nil || me != nil || ce != nil || !member || b.Start != strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10) {
			return out, errors.Join(ErrHeld, e, te, me, ce)
		}
		members = append(members, b)
	}
	after, e := windowsServicePIDsAllowEmpty(w.job, true)
	if e != nil || !reflect.DeepEqual(before, after) {
		return out, errors.Join(ErrHeld, e)
	}
	out = ServiceObservation{Version: ServiceVersion, HostBirth: w.birth, ServiceMembers: members, ObservedAt: time.Now().UTC()}
	return out, nil
}
func (w *windowsServiceWitness) WaitEmpty(ctx context.Context) error {
	if ctx == nil {
		return ErrHeld
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		observation, e := w.Observe()
		if e != nil {
			return e
		}
		if len(observation.ServiceMembers) == 0 {
			w.mu.Lock()
			if w.process == 0 {
				w.mu.Unlock()
				return ErrHeld
			}
			state, e := windows.WaitForSingleObject(w.process, 0)
			w.mu.Unlock()
			if e != nil {
				return e
			}
			if state != uint32(windows.WAIT_OBJECT_0) {
				return ErrHeld
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
