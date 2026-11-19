//go:build windows

package dispatch

import "path/filepath"

// launchOrchestratorDetached starts the supervisor through its own scheduled
// task (hev\orch-NAME), so it outlives the SSH session or round that
// launched it. Stop and retire remove the task; so does the supervisor when
// it exits.
func launchOrchestratorDetached(exe, name string) (bool, error) {
	dir := OrchestratorDir(name)
	t, err := newSupervisorTask(orchestratorTaskName(name), exe, []string{"orchestrate", "supervise", name}, dir, controlEnvironment())
	if err != nil {
		return true, err
	}
	t.Log = filepath.Join(dir, "supervisor.log")
	return true, startSupervisorTask(supervisorTasks, t, supervisorConfirmWait)
}
