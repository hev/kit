//go:build windows

package nativehost

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PROC_THREAD_ATTRIBUTE_JOB_LIST is documented for Windows 10+ and Server2016+.
// Unsupported attribute or nested-job restrictions hold before any resume.
const windowsJobListAttribute = uintptr(0x0002000d)

type windowsOwnedReceiver struct {
	process, job windows.Handle
	once         sync.Once
	result       error
}

// startReceiver constructs only the fixed pinned supervisor call. Context loss
// of a dispatch client is not authority to kill the accepted receiver.
func startReceiver(p Profile, descriptorRef string) (func() error, error) {
	if p.verify() != nil || !canonical(descriptorRef) {
		return nil, ErrHeld
	}
	owned, err := startWindowsOwned(p.SupervisorRef, []string{"--config", p.ConfigRef, "--supervise", descriptorRef}, p.Runtime.Directory, p.Runtime.Environment)
	if err != nil {
		return nil, err
	}
	return owned.wait, nil
}

func windowsEnvironment(env []string) ([]uint16, error) {
	seen := map[string]bool{}
	sorted := append([]string(nil), env...)
	for _, v := range sorted {
		key, value, ok := strings.Cut(v, "=")
		key = strings.ToUpper(key)
		if !ok || key == "" || seen[key] || strings.ContainsRune(v, 0) || !utf8.ValidString(value) {
			return nil, ErrHeld
		}
		seen[key] = true
	}
	sort.Slice(sorted, func(i, j int) bool { return strings.ToUpper(sorted[i]) < strings.ToUpper(sorted[j]) })
	block := utf16.Encode([]rune(strings.Join(sorted, "\x00")))
	return append(block, 0, 0), nil
}

// startWindowsOwned uses the ORIGINAL CreateProcess process/thread handles.
// Job membership is established by the kernel during suspended creation, never
// by opening/adopting an exec.Cmd PID after an unsuspended start. The job handle
// is noninheritable, unnamed and kill-on-close; breakaway flags are absent.
func startWindowsOwned(executable string, args []string, dir string, env []string) (*windowsOwnedReceiver, error) {
	app, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return nil, err
	}
	cwd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil, err
	}
	argv := append([]string{executable}, args...)
	line, err := windows.UTF16FromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return nil, err
	}
	environment, err := windowsEnvironment(env)
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	job, err := windows.CreateJobObject(&sa, nil)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*windowsOwnedReceiver, error) { return nil, errors.Join(err, windows.CloseHandle(job)) }
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fail(err)
	}
	// Standard handles are explicit NUL only; an allowlist prevents inheriting
	// ambient host credentials, private IPC, ownership-lock or job handles.
	nullName, _ := windows.UTF16PtrFromString("NUL")
	inherit := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	null, err := windows.CreateFile(nullName, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, &inherit, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(null)
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return fail(err)
	}
	defer attributes.Delete()
	jobs := []windows.Handle{job}
	handles := []windows.Handle{null}
	if err = attributes.Update(windowsJobListAttribute, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])); err != nil {
		return fail(err)
	}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), unsafe.Sizeof(handles[0])); err != nil {
		return fail(err)
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput = null
	startup.StdOutput = null
	startup.StdErr = null
	var child windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED)
	err = windows.CreateProcess(app, &line[0], nil, nil, true, flags, &environment[0], cwd, &startup.StartupInfo, &child)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(environment)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(handles)
	if err != nil {
		return fail(err)
	}
	abort := func(err error) (*windowsOwnedReceiver, error) {
		terminate := windows.TerminateJobObject(job, 1)
		_, waitErr := windows.WaitForSingleObject(child.Process, 10000)
		return nil, errors.Join(err, terminate, waitErr, windows.CloseHandle(child.Thread), windows.CloseHandle(child.Process), windows.CloseHandle(job))
	}
	// This is the exact primary thread returned by CreateProcess, not a thread
	// discovered through a PID/snapshot. Resume count must be exactly one.
	count, err := windows.ResumeThread(child.Thread)
	if err != nil || count != 1 {
		return abort(errors.Join(ErrHeld, err))
	}
	if err = windows.CloseHandle(child.Thread); err != nil {
		return abort(err)
	}
	return &windowsOwnedReceiver{process: child.Process, job: job}, nil
}

type windowsJobAccounting struct {
	TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
	PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
}

func windowsJobActive(job windows.Handle) (uint32, error) {
	var state windowsJobAccounting
	var returned uint32
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&state)), uint32(unsafe.Sizeof(state)), &returned)
	if err != nil {
		return 0, err
	}
	if returned < uint32(unsafe.Sizeof(state)) {
		return 0, ErrHeld
	}
	return state.ActiveProcesses, nil
}
func (o *windowsOwnedReceiver) wait() error {
	o.once.Do(func() {
		status, err := windows.WaitForSingleObject(o.process, windows.INFINITE)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			o.result = errors.Join(ErrHeld, err)
			return
		}
		var exitCode uint32
		if err = windows.GetExitCodeProcess(o.process, &exitCode); err != nil {
			o.result = errors.Join(ErrHeld, err)
			return
		}
		// Root exit alone never proves that the owned tree is gone. Terminate only
		// this retained job, then query its kernel accounting before releasing it.
		active, err := windowsJobActive(o.job)
		if err == nil && active != 0 {
			err = windows.TerminateJobObject(o.job, 1)
			deadline := time.Now().Add(10 * time.Second)
			for err == nil && active != 0 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
				active, err = windowsJobActive(o.job)
			}
		}
		if err != nil || active != 0 {
			o.result = errors.Join(ErrHeld, err)
			return
		}
		o.result = errors.Join(closeWindowsOwnedHandle(&o.process), closeWindowsOwnedHandle(&o.job))
		if exitCode != 0 {
			o.result = errors.Join(o.result, fmt.Errorf("native receiver exit code %d", exitCode))
		}
	})
	// Unknown tree state intentionally retains handles and returns Held. It must
	// not be converted to Gone, replay or a released capacity permit by callers.
	return o.result
}

func closeWindowsOwnedHandle(h *windows.Handle) error {
	if *h == 0 {
		return nil
	}
	err := windows.CloseHandle(*h)
	if err == nil {
		*h = 0
	}
	return err
}
