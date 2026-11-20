//go:build !windows

package dispatch

import "fmt"

// Supervisors run in tmux here; there is no Task Scheduler.
var supervisorTasks taskScheduler = noTaskScheduler{}

type noTaskScheduler struct{}

func (noTaskScheduler) Register(string, string) error {
	return fmt.Errorf("Task Scheduler requires Windows")
}
func (noTaskScheduler) Run(string) error    { return fmt.Errorf("Task Scheduler requires Windows") }
func (noTaskScheduler) Remove(string) error { return nil }
func (noTaskScheduler) Status(string) (taskStatus, error) {
	return taskStatus{}, fmt.Errorf("Task Scheduler requires Windows")
}
