//go:build windows

package services

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startProcessTree(cmd *exec.Cmd) (int, func(time.Duration, <-chan struct{}), func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	var closeOnce sync.Once
	closeJob := func() {
		closeOnce.Do(func() {
			_ = windows.CloseHandle(job)
		})
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		closeJob()
		return 0, nil, nil, err
	}

	// Start suspended in a new process group so children cannot escape Job
	// Object assignment and CTRL_BREAK_EVENT targets only this group.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		closeJob()
		return 0, nil, nil, err
	}
	pid := cmd.Process.Pid
	if err := assignAndResume(job, uint32(pid)); err != nil {
		_ = cmd.Process.Kill()
		closeJob()
		_ = cmd.Wait()
		return 0, nil, nil, err
	}

	gracefulStop := func(timeout time.Duration, waitDone <-chan struct{}) {
		if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)); err != nil {
			closeJob()
			<-waitDone
			return
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-waitDone:
		case <-timer.C:
		}
		// Closing the Job Object terminates any surviving child processes.
		closeJob()
		<-waitDone
	}
	return pid, gracefulStop, closeJob, nil
}

func assignAndResume(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(thread)
		_, err = windows.ResumeThread(thread)
		return err
	}
	return err
}
