//go:build windows

package nativejob

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errWorkloadOwnership = errors.New("Windows workload ownership uncertain")

// PROC_THREAD_ATTRIBUTE_JOB_LIST requires Windows 10+/Server2016+.
// Unsupported APIs or nested-job restrictions hold before resume.
const workloadJobListAttribute = uintptr(0x0002000d)

type windowsWorkload struct {
	process, job, thread windows.Handle
	once                 sync.Once
	mu                   sync.Mutex
	result               error
	exitCode             int
	birth                Birth
}

func workloadEnvironment(env []string) ([]uint16, error) {
	seen := map[string]bool{}
	sorted := append([]string(nil), env...)
	for _, v := range sorted {
		key, value, ok := strings.Cut(v, "=")
		key = strings.ToUpper(key)
		if !ok || key == "" || seen[key] || strings.ContainsRune(v, 0) || !utf8.ValidString(value) {
			return nil, errWorkloadOwnership
		}
		seen[key] = true
	}
	sort.Slice(sorted, func(i, j int) bool { return strings.ToUpper(sorted[i]) < strings.ToUpper(sorted[j]) })
	block := utf16.Encode([]rune(strings.Join(sorted, "\x00")))
	return append(block, 0, 0), nil
}

// startWindowsWorkload uses the ORIGINAL CreateProcess process/thread handles.
// Job membership is established by the kernel during suspended creation, never
// by opening/adopting an exec.Cmd PID after an unsuspended start. The job handle
// is noninheritable, unnamed and kill-on-close; breakaway flags are absent.
func startWindowsWorkload(cmd *exec.Cmd) (*windowsWorkload, error) {
	if cmd == nil || cmd.Err != nil || cmd.Process != nil || cmd.ProcessState != nil || cmd.SysProcAttr != nil || len(cmd.ExtraFiles) != 0 || cmd.Stdin != nil || cmd.Cancel != nil || len(cmd.Args) == 0 || cmd.Env == nil {
		return nil, errWorkloadOwnership
	}
	stdout, ok := cmd.Stdout.(*os.File)
	if !ok || stdout == nil {
		return nil, errWorkloadOwnership
	}
	stderr, ok := cmd.Stderr.(*os.File)
	if !ok || stderr == nil {
		return nil, errWorkloadOwnership
	}
	executable, args, dir, env := cmd.Path, cmd.Args[1:], cmd.Dir, cmd.Env

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
	environment, err := workloadEnvironment(env)
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
	fail := func(err error) (*windowsWorkload, error) { return nil, errors.Join(err, windows.CloseHandle(job)) }
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return fail(err)
	}
	// Only NUL stdin and duplicated owned artifact handles are inherited.
	// Ambient host IPC, locks, credentials and job handles are excluded.
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
	var out, errout windows.Handle
	current := windows.CurrentProcess()
	if err = windows.DuplicateHandle(current, windows.Handle(stdout.Fd()), current, &out, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(out)
	if err = windows.DuplicateHandle(current, windows.Handle(stderr.Fd()), current, &errout, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(errout)
	handles := []windows.Handle{null, out, errout}
	if err = attributes.Update(workloadJobListAttribute, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])); err != nil {
		return fail(err)
	}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return fail(err)
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput = null
	startup.StdOutput = out
	startup.StdErr = errout
	var child windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED)
	err = windows.CreateProcess(app, &line[0], nil, nil, true, flags, &environment[0], cwd, &startup.StartupInfo, &child)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(sd)
	runtime.KeepAlive(environment)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(handles)
	if err != nil {
		return fail(err)
	}
	owned := &windowsWorkload{process: child.Process, job: job, thread: child.Thread}
	abort := func(cause error) (*windowsWorkload, error) {
		terminate := windows.TerminateJobObject(job, 1)
		status, waitErr := windows.WaitForSingleObject(child.Process, 10000)
		active, queryErr := workloadJobActive(job)
		if terminate != nil || waitErr != nil || status != windows.WAIT_OBJECT_0 || queryErr != nil || active != 0 {
			return owned, errors.Join(errWorkloadOwnership, cause, terminate, waitErr, queryErr)
		}
		closeErr := errors.Join(closeWorkloadHandle(&owned.thread), closeWorkloadHandle(&owned.process), closeWorkloadHandle(&owned.job))
		return nil, errors.Join(cause, closeErr)
	}
	birth, err := workloadOriginalBirth(child.Process, child.ProcessId)
	if err != nil {
		return abort(err)
	}
	owned.birth = birth
	// This is the exact primary thread returned by CreateProcess, not a thread
	// discovered through a PID/snapshot. Resume count must be exactly one.
	count, err := windows.ResumeThread(child.Thread)
	if err != nil || count != 1 {
		return abort(errors.Join(errWorkloadOwnership, err))
	}
	if err = closeWorkloadHandle(&owned.thread); err != nil {
		return abort(err)
	}
	return owned, nil
}

type workloadJobAccounting struct {
	TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
	PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
}

func workloadJobActive(job windows.Handle) (uint32, error) {
	var state workloadJobAccounting
	var returned uint32
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&state)), uint32(unsafe.Sizeof(state)), &returned)
	if err != nil {
		return 0, err
	}
	if returned < uint32(unsafe.Sizeof(state)) {
		return 0, errWorkloadOwnership
	}
	return state.ActiveProcesses, nil
}
func (o *windowsWorkload) Birth() (Birth, error) {
	if o.birth.PID == 0 || o.birth.BootID == "" || o.birth.Start == "" {
		return Birth{}, errWorkloadOwnership
	}
	return o.birth, nil
}
func (o *windowsWorkload) Kill() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.job == 0 {
		return os.ErrProcessDone
	}
	return windows.TerminateJobObject(o.job, 1)
}
func (o *windowsWorkload) Wait() (int, error) {
	o.once.Do(func() {
		status, err := windows.WaitForSingleObject(o.process, windows.INFINITE)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			o.result = errors.Join(errWorkloadOwnership, err)
			return
		}
		var exitCode uint32
		if err = windows.GetExitCodeProcess(o.process, &exitCode); err != nil {
			o.result = errors.Join(errWorkloadOwnership, err)
			return
		}
		// Root exit alone never proves that the owned tree is gone. Terminate only
		// this retained job, then query its kernel accounting before releasing it.
		active, err := workloadJobActive(o.job)
		if err == nil && active != 0 {
			err = windows.TerminateJobObject(o.job, 1)
			deadline := time.Now().Add(10 * time.Second)
			for err == nil && active != 0 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
				active, err = workloadJobActive(o.job)
			}
		}
		if err != nil || active != 0 {
			o.result = errors.Join(errWorkloadOwnership, err)
			return
		}
		o.mu.Lock()
		o.result = errors.Join(closeWorkloadHandle(&o.thread), closeWorkloadHandle(&o.process), closeWorkloadHandle(&o.job))
		o.mu.Unlock()
		o.exitCode = int(exitCode)
	})
	// Unknown tree state intentionally retains handles and returns uncertainty. It must
	// not be converted to Gone, replay or a released capacity permit by callers.
	return o.exitCode, o.result
}

func closeWorkloadHandle(h *windows.Handle) error {
	if *h == 0 {
		return nil
	}
	err := windows.CloseHandle(*h)
	if err == nil {
		*h = 0
	}
	return err
}

// Birth is captured from the original suspended process handle before resume.
// No process/thread lookup, snapshot or PID-only adoption is used.
func workloadOriginalBirth(handle windows.Handle, pid uint32) (Birth, error) {
	var boot struct {
		ID       windows.GUID
		Firmware uint32
		Padding  uint32
		Flags    uint64
	}
	var size uint32
	if err := windows.NtQuerySystemInformation(windows.SystemBootEnvironmentInformation, unsafe.Pointer(&boot), uint32(unsafe.Sizeof(boot)), &size); err != nil {
		return Birth{}, err
	}
	if size < 16 || boot.ID == (windows.GUID{}) {
		return Birth{}, errWorkloadOwnership
	}
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return Birth{}, err
	}
	ticks := uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	if ticks == 0 || pid == 0 {
		return Birth{}, errWorkloadOwnership
	}
	return Birth{BootID: boot.ID.String(), PID: int(pid), Start: strconv.FormatUint(ticks, 10)}, nil
}
