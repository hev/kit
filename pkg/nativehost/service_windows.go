//go:build windows

package nativehost

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	fs "github.com/hev/kit/pkg/nativefs/windowsfs"
	n "github.com/hev/kit/pkg/nativejob"
	"golang.org/x/sys/windows"
)

var serviceCreateJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateJobObjectW")
var serviceOpenJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")
var serviceInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

func serviceSum(value string) string {
	b := sha256.Sum256([]byte(value))
	return hex.EncodeToString(b[:])
}
func windowsServiceName(path string, birth n.Birth) (string, error) {
	if fs.PathSyntax(path) != nil || birth.PID <= 1 || birth.BootID == "" || birth.Start == "" {
		return "", ErrHeld
	}
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", e
	}
	// Local namespace is deliberately the existing user session. A Session0
	// controller is not an observer of the supported Session2 coding service.
	return `Local\hev-nativehost-service-v1-` + serviceSum(user.User.Sid.String()) + "-" + serviceSum(strings.ToLower(path)) + "-" + serviceSum(n.Digest(birth)), nil
}
func windowsServiceContains(job, process windows.Handle) (bool, error) {
	var result int32
	ok, _, e := serviceInJob.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&result)))
	if ok == 0 {
		return false, e
	}
	return result != 0, nil
}
func windowsServiceLimits(job windows.Handle) error {
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	var returned uint32
	if e := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), &returned); e != nil {
		return e
	}
	if returned < uint32(unsafe.Sizeof(limits)) || limits.BasicLimitInformation.LimitFlags&(windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK|windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK) != 0 {
		return ErrHeld
	}
	return nil
}
func windowsServicePIDs(job windows.Handle) ([]uint32, error) {
	var list struct {
		Assigned, Count uint32
		PIDs            [1024]uintptr
	}
	var returned uint32
	if e := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), &returned); e != nil {
		return nil, e
	}
	if returned < 8 || list.Count == 0 || list.Count != list.Assigned || list.Count > uint32(len(list.PIDs)) || uint64(returned) < 8+uint64(list.Count)*uint64(unsafe.Sizeof(uintptr(0))) {
		return nil, ErrHeld
	}
	out := make([]uint32, 0, list.Count)
	seen := map[uint32]bool{}
	for _, id := range list.PIDs[:list.Count] {
		if id <= 1 || uint64(id) > uint64(^uint32(0)) || seen[uint32(id)] {
			return nil, ErrHeld
		}
		seen[uint32(id)] = true
		out = append(out, uint32(id))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
func platformEnterService(path string, expected n.Birth) (close func() error, err error) {
	if !supported() {
		return nil, ErrHeld
	}
	session, e := windowsUserSession()
	if e != nil || session != 2 {
		return nil, ErrHeld
	}
	actual, e := n.ProcessBirth(os.Getpid())
	if e != nil || actual != expected {
		return nil, ErrHeld
	}
	if fs.VerifyPrivate(path, true, false) != nil {
		return nil, ErrHeld
	}
	name, e := windowsServiceName(path, expected)
	if e != nil {
		return nil, e
	}
	ptr, e := windows.UTF16PtrFromString(name)
	if e != nil {
		return nil, e
	}
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return nil, e
	}
	// Same protected token-user kernel-object policy as the owned receiver Jobs.
	sd, e := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;;GA;;;" + user.User.Sid.String() + ")")
	if e != nil {
		return nil, e
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	// LazyProc.Call captures LastError from this exact native call even on a
	// successful existing-name open. Existing objects are never adopted.
	handle, _, callErr := serviceCreateJob.Call(uintptr(unsafe.Pointer(&sa)), uintptr(unsafe.Pointer(ptr)))
	runtime.KeepAlive(sd)
	if handle == 0 {
		return nil, callErr
	}
	job := windows.Handle(handle)
	if errors.Is(callErr, windows.ERROR_ALREADY_EXISTS) {
		return nil, errors.Join(ErrHeld, windows.CloseHandle(job))
	}
	var once sync.Once
	var closeErr error
	release := func() error { once.Do(func() { closeErr = windows.CloseHandle(job) }); return closeErr }
	fail := func(e error) (func() error, error) { return nil, errors.Join(e, release()) }
	if e = fs.VerifyPrivateJobHandle(job); e != nil {
		return fail(e)
	}
	// No breakaway and no kill-on-close: closing an observation owner is never
	// converted into termination authority or a claim that its descendants died.
	if e = windowsServiceLimits(job); e != nil {
		return fail(e)
	}
	current := windows.CurrentProcess()
	if e = windows.AssignProcessToJobObject(job, current); e != nil {
		return fail(e)
	}
	// Once assigned, retain the real original Job handle even if any later
	// validation fails. The shared owner must close it on its own exit path.
	close = release
	member, e := windowsServiceContains(job, current)
	if e != nil || !member {
		return close, errors.Join(ErrHeld, e)
	}
	pids, e := windowsServicePIDs(job)
	if e != nil || len(pids) != 1 || pids[0] != uint32(expected.PID) {
		return close, errors.Join(ErrHeld, e)
	}
	after, e := n.ProcessBirth(os.Getpid())
	if e != nil || after != expected {
		return close, errors.Join(ErrHeld, e)
	}
	return close, nil
}
func platformObserveService(expected n.Birth, path string) (out ServiceObservation, err error) {
	if !supported() {
		return out, ErrHeld
	}
	session, e := windowsUserSession()
	if e != nil || session != 2 {
		return out, ErrHeld
	}
	if fs.VerifyPrivate(path, true, false) != nil {
		return out, ErrHeld
	}
	name, e := windowsServiceName(path, expected)
	if e != nil {
		return out, e
	}
	ptr, e := windows.UTF16PtrFromString(name)
	if e != nil {
		return out, e
	}
	const queryAndReadControl = uint32(0x0004 | 0x00020000)
	raw, _, e := serviceOpenJob.Call(uintptr(queryAndReadControl), 0, uintptr(unsafe.Pointer(ptr)))
	if raw == 0 {
		return out, e
	}
	job := windows.Handle(raw)
	defer func() { err = errors.Join(err, windows.CloseHandle(job)) }()
	if e = fs.VerifyPrivateJobHandle(job); e != nil {
		return out, e
	}
	if e = windowsServiceLimits(job); e != nil {
		return out, e
	}
	process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(expected.PID))
	if e != nil {
		return out, e
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(process)) }()
	member, e := windowsServiceContains(job, process)
	if e != nil || !member {
		return out, ErrHeld
	}
	var token windows.Token
	if e = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); e != nil {
		return out, e
	}
	defer func() { err = errors.Join(err, token.Close()) }()
	user, e := token.GetTokenUser()
	if e != nil {
		return out, e
	}
	own, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil || !windows.EqualSid(user.User.Sid, own.User.Sid) {
		return out, ErrHeld
	}
	var hostSession uint32
	if e = windows.ProcessIdToSessionId(uint32(expected.PID), &hostSession); e != nil || hostSession != session {
		return out, ErrHeld
	}
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if e = windows.QueryFullProcessImageName(process, 0, &image[0], &size); e != nil {
		return out, e
	}
	expectedImage := filepath.Join(filepath.Dir(filepath.Dir(path)), "bin", "hev.exe")
	if !strings.EqualFold(windows.UTF16ToString(image[:size]), expectedImage) {
		return out, ErrHeld
	}
	var creation, exit, kernel, userTime windows.Filetime
	if e = windows.GetProcessTimes(process, &creation, &exit, &kernel, &userTime); e != nil {
		return out, e
	}
	ticks := uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	if strconv.FormatUint(ticks, 10) != expected.Start {
		return out, ErrHeld
	}
	birth, e := n.ProcessBirth(expected.PID)
	if e != nil || birth != expected {
		return out, ErrHeld
	}
	before, e := windowsServicePIDs(job)
	if e != nil {
		return out, e
	}
	members := make([]n.Birth, 0, len(before))
	for _, pid := range before {
		b, e := n.ProcessBirth(int(pid))
		if e != nil {
			return out, e
		}
		members = append(members, b)
	}
	after, e := windowsServicePIDs(job)
	if e != nil || !reflect.DeepEqual(before, after) {
		return out, ErrHeld
	}
	birth, e = n.ProcessBirth(expected.PID)
	if e != nil || birth != expected {
		return out, ErrHeld
	}
	state, e := windows.WaitForSingleObject(process, 0)
	if e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return out, ErrHeld
	}
	out = ServiceObservation{Version: ServiceVersion, HostBirth: expected, ServiceMembers: members, ObservedAt: time.Now().UTC()}
	return out, nil
}
