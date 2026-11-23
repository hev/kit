package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/hev/kit/internal/attention"
	"github.com/hev/kit/internal/platformops"
	"github.com/hev/kit/internal/secrets"
)

const (
	DefaultInitialBackoff = 2 * time.Second
	DefaultMaxBackoff     = 5 * time.Minute
	DefaultCrashWindow    = 2 * time.Minute
	DefaultHealthyUptime  = 60 * time.Second
	DefaultPollInterval   = 100 * time.Millisecond
	CrashLoopThreshold    = 3
)

type Supervisor struct {
	Home       string
	Membership Membership
	Resolve    func(context.Context, []secrets.Ref) (map[string]string, error)
	Notice     func(context.Context, attention.Notice) error
	Logger     *slog.Logger

	// Optional overrides for deterministic tests; zero uses production defaults.
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	CrashWindow    time.Duration
	HealthyUptime  time.Duration
	PollInterval   time.Duration
}

func (s Supervisor) initialBackoff() time.Duration {
	if s.InitialBackoff > 0 {
		return s.InitialBackoff
	}
	return DefaultInitialBackoff
}

func (s Supervisor) maxBackoff() time.Duration {
	if s.MaxBackoff > 0 {
		return s.MaxBackoff
	}
	return DefaultMaxBackoff
}

func (s Supervisor) crashWindow() time.Duration {
	if s.CrashWindow > 0 {
		return s.CrashWindow
	}
	return DefaultCrashWindow
}

func (s Supervisor) healthyUptime() time.Duration {
	if s.HealthyUptime > 0 {
		return s.HealthyUptime
	}
	return DefaultHealthyUptime
}

func (s Supervisor) pollInterval() time.Duration {
	if s.PollInterval > 0 {
		return s.PollInterval
	}
	return DefaultPollInterval
}

func (s Supervisor) dir(svc Service) string {
	return filepath.Join(servicesRoot(s.Home), svc.Name)
}

// BackoffDuration computes bounded exponential backoff (2s -> 4s -> 8s ... -> 5m cap).
func BackoffDuration(attempt int, initial, maximum time.Duration) time.Duration {
	if initial <= 0 {
		initial = DefaultInitialBackoff
	}
	if maximum < initial {
		maximum = initial
	}
	d := initial
	for i := 1; i < attempt && d < maximum; i++ {
		if d > maximum/2 {
			return maximum
		}
		d *= 2
	}
	if d > maximum {
		return maximum
	}
	return d
}

// Start supervises configured services until ctx ends or the returned stop
// function is called. Stopping waits for all child process groups to terminate.
func (s Supervisor) Start(ctx context.Context, configured []Service) (func(), error) {
	if len(configured) == 0 {
		return func() {}, nil
	}
	root := servicesRoot(s.Home)
	if err := platformops.MkdirAll(root, 0700); err != nil {
		return nil, errStorage
	}
	releaseSup, err := platformops.LockPath(supervisorLockPath(s.Home), true)
	if err != nil {
		if platformops.IsLockBusy(err) {
			return nil, errors.New("another service supervisor is already running")
		}
		return nil, errStorage
	}

	supCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for _, svc := range configured {
		wg.Go(func() {
			s.supervise(supCtx, svc)
		})
	}
	stop := sync.OnceFunc(func() {
		cancel()
		wg.Wait()
		releaseSup()
	})
	return stop, nil
}

func (s Supervisor) saveState(dir string, st State) {
	release, err := lockState(context.Background(), dir)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("could not lock service state", "dir", filepath.Base(dir))
		}
		return
	}
	defer release()
	if err := writeState(dir, s.Home, st); err != nil && s.Logger != nil {
		s.Logger.Warn("could not save service state", "dir", filepath.Base(dir))
	}
}

func (s Supervisor) popControl(dir string) (controlRequest, bool) {
	ctrlPath := filepath.Join(dir, "control.json")
	if _, err := os.Stat(ctrlPath); err != nil {
		return controlRequest{}, false
	}
	release, err := lockState(context.Background(), dir)
	if err != nil {
		return controlRequest{}, false
	}
	defer release()
	var req controlRequest
	if err := readJSON(ctrlPath, &req); err != nil {
		_ = os.Remove(ctrlPath)
		return controlRequest{}, false
	}
	_ = os.Remove(ctrlPath)
	return req, req.ID != ""
}

func (s Supervisor) writeAck(dir string, st State, ack controlAck) {
	release, err := lockState(context.Background(), dir)
	if err != nil {
		return
	}
	defer release()
	ack.Error = scrubReason(ack.Error, s.Home, dir)
	_ = writeState(dir, s.Home, st)
	_ = writeJSON(filepath.Join(dir, "ack.json"), ack)
}

func (s Supervisor) resolveEnv(ctx context.Context, svc Service) (map[string]string, error) {
	if len(svc.EnvRefs) == 0 {
		return nil, nil
	}
	if s.Resolve == nil {
		return nil, errors.New("could not resolve service secret references")
	}
	refs := make([]secrets.Ref, 0, len(svc.EnvRefs))
	for name, uri := range svc.EnvRefs {
		refs = append(refs, secrets.Ref{Name: name, URI: uri, TTL: 5 * time.Minute})
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	values, err := s.Resolve(resolveCtx, refs)
	if err != nil {
		return nil, errors.New("could not resolve service secret references")
	}
	return values, nil
}

func (s Supervisor) emitCrashNotice(ctx context.Context, svc Service, episode string, exits int) {
	if s.Notice == nil {
		return
	}
	host := s.Membership.Self
	if host == "" {
		host, _ = os.Hostname()
	}
	text := fmt.Sprintf("Service crashed %d times within %s", exits, s.crashWindow())
	notice := attention.New(attention.KindDone, attention.SourceService, host, svc.Name,
		episode, StatusFailed, text, attention.Links{Run: "hev service show " + svc.Name})
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if err := s.Notice(notifyCtx, notice); err != nil && s.Logger != nil {
		s.Logger.Warn("could not emit service attention notice", "service", svc.Name)
	}
}

func (s Supervisor) supervise(ctx context.Context, svc Service) {
	dir := s.dir(svc)
	var st State
	if _, err := os.Stat(dir); err == nil {
		if release, lerr := lockState(ctx, dir); lerr == nil {
			st, _ = readState(dir)
			release()
		}
	}
	st.PID = 0
	st.NextRestart = nil
	if st.Status == "" || st.Status == StatusRunning || st.Status == StatusBackoff {
		st.Status = StatusStopped
	}

	var (
		proc           *runningProcess
		healthyReached bool
		streak         int
		rapidExits     []time.Time
		backoffUntil   time.Time
		terminal       bool
		healthyTimer   *time.Timer
		backoffTimer   *time.Timer
	)

	stopHealthyTimer := func() {
		if healthyTimer != nil {
			healthyTimer.Stop()
			healthyTimer = nil
		}
	}
	stopBackoffTimer := func() {
		if backoffTimer != nil {
			backoffTimer.Stop()
			backoffTimer = nil
		}
	}
	defer func() {
		stopHealthyTimer()
		stopBackoffTimer()
	}()

	resetCrashTracking := func() {
		streak = 0
		rapidExits = nil
		backoffUntil = time.Time{}
		stopBackoffTimer()
		st.NextRestart = nil
		st.CrashLoop = false
		st.CrashEpisode = ""
	}

	stopCurrentProcess := func() {
		if proc == nil {
			return
		}
		stopHealthyTimer()
		proc.stop(svc.StopTimeoutDuration())
		res := <-proc.done
		proc.flushLog()
		proc = nil
		now := time.Now().UTC()
		st.PID = 0
		st.LastStop = &now
		if res.exitCode != nil {
			st.LastExit = res.exitCode
		}
	}

	recordRapidFailure := func(now time.Time, reason string) {
		window := s.crashWindow()
		kept := rapidExits[:0]
		for _, t := range rapidExits {
			if now.Sub(t) <= window {
				kept = append(kept, t)
			}
		}
		rapidExits = append(kept, now)
		if len(rapidExits) >= CrashLoopThreshold {
			st.CrashLoop = true
			if st.CrashEpisode == "" {
				st.CrashEpisode = rapidExits[0].UTC().Format(time.RFC3339Nano)
				s.saveState(dir, st)
				s.emitCrashNotice(ctx, svc, st.CrashEpisode, len(rapidExits))
			}
		}
		if svc.RestartMode() == RestartNever {
			terminal = true
			st.Status = StatusFailed
			st.Reason = reason
			st.NextRestart = nil
			s.saveState(dir, st)
			return
		}
		streak++
		delay := BackoffDuration(streak, s.initialBackoff(), s.maxBackoff())
		next := now.Add(delay)
		backoffUntil = next
		stopBackoffTimer()
		backoffTimer = time.NewTimer(delay)
		st.Status = StatusBackoff
		st.NextRestart = &next
		if reason != "" {
			st.Reason = reason
		} else if st.CrashLoop {
			st.Reason = "crash loop: backing off"
		} else {
			st.Reason = ""
		}
		s.saveState(dir, st)
	}

	launchNow := func(isAutoRestart bool) error {
		values, err := s.resolveEnv(ctx, svc)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if isAutoRestart {
				st.Restarts++
			}
			recordRapidFailure(time.Now().UTC(), err.Error())
			return err
		}
		p, err := startServiceProcess(svc, s.Home, dir, values)
		if err != nil {
			if isAutoRestart {
				st.Restarts++
			}
			recordRapidFailure(time.Now().UTC(), err.Error())
			return err
		}
		if isAutoRestart {
			st.Restarts++
		}
		proc = p
		healthyReached = false
		startedUTC := p.started.UTC()
		st.Status = StatusRunning
		st.PID = p.pid
		st.StartTime = &startedUTC
		st.NextRestart = nil
		st.Reason = ""
		stopHealthyTimer()
		healthyTimer = time.NewTimer(s.healthyUptime())
		s.saveState(dir, st)
		return nil
	}

	reconcile := func() {
		if ctx.Err() != nil {
			return
		}
		winner, reason := s.Membership.winner(ctx, svc)
		switch {
		case !svc.enabled():
			wasRunning := proc != nil
			stopCurrentProcess()
			stopBackoffTimer()
			backoffUntil = time.Time{}
			if wasRunning || st.Status != StatusStopped || st.Reason != "disabled in configuration" || st.PlacementWinner != winner {
				st.Status = StatusStopped
				st.Reason = "disabled in configuration"
				st.PlacementWinner = winner
				st.NextRestart = nil
				s.saveState(dir, st)
			}
			return
		case st.Stopped:
			wasRunning := proc != nil
			stopCurrentProcess()
			stopBackoffTimer()
			backoffUntil = time.Time{}
			if wasRunning || st.Status != StatusStopped || st.Reason != "stopped locally" || st.PlacementWinner != winner {
				st.Status = StatusStopped
				st.Reason = "stopped locally"
				st.PlacementWinner = winner
				st.NextRestart = nil
				s.saveState(dir, st)
			}
			return
		case reason != "":
			wasRunning := proc != nil
			stopCurrentProcess()
			terminal = false
			stopBackoffTimer()
			backoffUntil = time.Time{}
			streak = 0
			if wasRunning || st.Status != StatusStandby || st.Reason != reason || st.PlacementWinner != winner {
				st.Status = StatusStandby
				st.Reason = reason
				st.PlacementWinner = winner
				st.NextRestart = nil
				st.PID = 0
				s.saveState(dir, st)
			}
			return
		}

		// Eligible to run on this host.
		if st.PlacementWinner != winner {
			st.PlacementWinner = winner
			if proc != nil || terminal {
				s.saveState(dir, st)
			}
		}
		if proc != nil || terminal {
			return
		}
		if !backoffUntil.IsZero() && time.Now().Before(backoffUntil) {
			return
		}
		isAutoRestart := !backoffUntil.IsZero()
		backoffUntil = time.Time{}
		stopBackoffTimer()
		st.NextRestart = nil
		_ = launchNow(isAutoRestart)
	}

	handleControl := func(req controlRequest) {
		switch req.Action {
		case "stop":
			stopCurrentProcess()
			terminal = false
			backoffUntil = time.Time{}
			stopBackoffTimer()
			st.NextRestart = nil
			st.Stopped = true
			st.Status = StatusStopped
			st.Reason = "stopped locally"
			s.writeAck(dir, st, controlAck{ID: req.ID, Status: st.Status})
		case "start":
			if !svc.enabled() {
				s.writeAck(dir, st, controlAck{ID: req.ID, Error: "service is disabled in configuration"})
				return
			}
			st.Stopped = false
			terminal = false
			resetCrashTracking()
			winner, reason := s.Membership.winner(ctx, svc)
			st.PlacementWinner = winner
			if reason != "" {
				stopCurrentProcess()
				st.Status = StatusStandby
				st.Reason = reason
				s.writeAck(dir, st, controlAck{ID: req.ID, Status: st.Status, Error: "service is on standby: " + reason})
				return
			}
			if proc != nil {
				s.writeAck(dir, st, controlAck{ID: req.ID, Status: StatusRunning})
				return
			}
			if err := launchNow(false); err != nil {
				s.writeAck(dir, st, controlAck{ID: req.ID, Status: st.Status, Error: err.Error()})
				return
			}
			s.writeAck(dir, st, controlAck{ID: req.ID, Status: StatusRunning})
		case "restart":
			if !svc.enabled() {
				s.writeAck(dir, st, controlAck{ID: req.ID, Error: "service is disabled in configuration"})
				return
			}
			stopCurrentProcess()
			st.Stopped = false
			terminal = false
			resetCrashTracking()
			winner, reason := s.Membership.winner(ctx, svc)
			st.PlacementWinner = winner
			if reason != "" {
				st.Status = StatusStandby
				st.Reason = reason
				s.writeAck(dir, st, controlAck{ID: req.ID, Status: st.Status, Error: "service is on standby: " + reason})
				return
			}
			if err := launchNow(false); err != nil {
				s.writeAck(dir, st, controlAck{ID: req.ID, Status: st.Status, Error: err.Error()})
				return
			}
			s.writeAck(dir, st, controlAck{ID: req.ID, Status: StatusRunning})
		default:
			s.writeAck(dir, st, controlAck{ID: req.ID, Error: "unknown service control action"})
		}
	}

	reconcile()
	poll := time.NewTicker(s.pollInterval())
	defer poll.Stop()

	for {
		var procDone <-chan procResult
		if proc != nil {
			procDone = proc.done
		}
		var healthyCh <-chan time.Time
		if healthyTimer != nil {
			healthyCh = healthyTimer.C
		}
		var backoffCh <-chan time.Time
		if backoffTimer != nil {
			backoffCh = backoffTimer.C
		}

		select {
		case <-ctx.Done():
			stopCurrentProcess()
			stopBackoffTimer()
			st.NextRestart = nil
			st.PID = 0
			if st.Status == StatusRunning || st.Status == StatusBackoff {
				st.Status = StatusStopped
				st.Reason = ""
			}
			s.saveState(dir, st)
			return

		case res := <-procDone:
			stopHealthyTimer()
			uptime := time.Since(proc.started)
			wasHealthy := healthyReached || uptime > s.healthyUptime()
			now := time.Now().UTC()
			proc.flushLog()
			proc = nil
			healthyReached = false
			st.PID = 0
			st.LastStop = &now
			st.LastExit = res.exitCode

			if wasHealthy {
				resetCrashTracking()
			}

			winner, placeReason := s.Membership.winner(ctx, svc)
			st.PlacementWinner = winner
			if !svc.enabled() {
				st.Status = StatusStopped
				st.Reason = "disabled in configuration"
				s.saveState(dir, st)
				continue
			}
			if st.Stopped {
				st.Status = StatusStopped
				st.Reason = "stopped locally"
				s.saveState(dir, st)
				continue
			}
			if placeReason != "" {
				terminal = false
				st.Status = StatusStandby
				st.Reason = placeReason
				s.saveState(dir, st)
				continue
			}

			cleanZero := res.err == nil && res.exitCode != nil && *res.exitCode == 0
			switch svc.RestartMode() {
			case RestartNever:
				terminal = true
				if cleanZero {
					st.Status = StatusStopped
					st.Reason = ""
					s.saveState(dir, st)
				} else {
					recordRapidFailure(now, res.reason)
				}
			case RestartOnFailure:
				if cleanZero {
					terminal = true
					st.Status = StatusStopped
					st.Reason = ""
					s.saveState(dir, st)
				} else {
					recordRapidFailure(now, res.reason)
				}
			default: // RestartAlways
				if wasHealthy {
					// Healthy long-lived run exited: still apply initial 2s backoff
					// without counting as a rapid exit before first crash.
					streak = 1
					delay := BackoffDuration(streak, s.initialBackoff(), s.maxBackoff())
					next := now.Add(delay)
					backoffUntil = next
					stopBackoffTimer()
					backoffTimer = time.NewTimer(delay)
					st.Status = StatusBackoff
					st.NextRestart = &next
					st.Reason = res.reason
					s.saveState(dir, st)
				} else {
					recordRapidFailure(now, res.reason)
				}
			}

		case <-healthyCh:
			healthyTimer = nil
			if proc != nil {
				healthyReached = true
				wasCrashLoop := st.CrashLoop || st.CrashEpisode != ""
				resetCrashTracking()
				if wasCrashLoop {
					s.saveState(dir, st)
				}
			}

		case <-backoffCh:
			backoffTimer = nil
			reconcile()

		case <-poll.C:
			if req, ok := s.popControl(dir); ok {
				handleControl(req)
			}
			reconcile()
		}
	}
}

func (s Supervisor) Inspect(ctx context.Context, svc Service, logLines int) (Details, error) {
	winner, placeReason := s.Membership.winner(ctx, svc)
	liveSup := supervisorRunning(s.Home)

	var envKeys []string
	if len(svc.EnvRefs) > 0 {
		envKeys = make([]string, 0, len(svc.EnvRefs))
		for k := range svc.EnvRefs {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
	}

	d := Details{
		Summary: Summary{
			Name:            svc.Name,
			Status:          StatusStopped,
			Restart:         svc.RestartMode(),
			Placement:       svc.Placement,
			Enabled:         svc.enabled(),
			PlacementWinner: winner,
		},
		StopTimeout: svc.StopTimeout,
		EnvKeys:     envKeys,
	}

	dir := s.dir(svc)
	// Reading an unused service must not create its state directory.
	if _, err := os.Stat(dir); err == nil {
		release, err := lockState(ctx, dir)
		if err != nil {
			return d, err
		}
		st, err := readState(dir)
		release()
		if err != nil {
			return d, err
		}
		d.State = st
		if logLines != 0 {
			lines := logLines
			if lines < 0 {
				lines = 0
			}
			logs, err := readServiceLogs(dir, lines, s.Home, dir)
			if err != nil {
				return d, err
			}
			d.Logs = logs
		}
	} else if !os.IsNotExist(err) {
		return d, errStorage
	}

	st := d.State
	switch {
	case !svc.enabled():
		st.Status = StatusStopped
		st.Reason = "disabled in configuration"
		st.PID = 0
		st.NextRestart = nil
	case !liveSup:
		st.PID = 0
		st.NextRestart = nil
		if st.Stopped {
			st.Status = StatusStopped
			st.Reason = "stopped locally"
		} else if placeReason != "" {
			st.Status = StatusStandby
			st.Reason = placeReason
		} else if st.Status != StatusFailed {
			st.Status = StatusStopped
			if st.Reason == "" {
				st.Reason = "hevd is not running"
			}
		}
	default:
		if st.Status == "" {
			if placeReason != "" {
				st.Status = StatusStandby
				st.Reason = placeReason
			} else {
				st.Status = StatusStopped
			}
		}
	}
	if winner != "" {
		st.PlacementWinner = winner
	}
	d.State = st
	d.Status = st.Status
	d.Reason = st.Reason
	d.PID = st.PID
	d.StartTime = st.StartTime
	d.Restarts = st.Restarts
	d.LastExit = st.LastExit
	d.PlacementWinner = st.PlacementWinner
	d.NextRestart = st.NextRestart
	return d, nil
}

func (s Supervisor) Logs(svc Service, lines int) (string, error) {
	dir := s.dir(svc)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", errStorage
	}
	return readServiceLogs(dir, lines, s.Home, dir)
}

func (s Supervisor) Control(ctx context.Context, svc Service, action string) (State, error) {
	switch action {
	case "start", "stop", "restart":
	default:
		return State{}, errors.New("unknown service control action")
	}
	if !supervisorRunning(s.Home) {
		return State{}, errors.New("hevd service supervisor is not running; start hevd")
	}
	dir := s.dir(svc)
	waitBudget := svc.StopTimeoutDuration() + 10*time.Second
	releaseCtrl, err := lockPathWithTimeout(ctx, dir, "control.lock", waitBudget)
	if err != nil {
		return State{}, err
	}
	defer releaseCtrl()

	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return State{}, errStorage
	}
	req := controlRequest{
		ID:          hex.EncodeToString(nonce[:]),
		Action:      action,
		RequestedAt: time.Now().UTC(),
	}

	releaseState, err := lockState(ctx, dir)
	if err != nil {
		return State{}, err
	}
	_ = os.Remove(filepath.Join(dir, "ack.json"))
	err = writeJSON(filepath.Join(dir, "control.json"), req)
	releaseState()
	if err != nil {
		return State{}, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, waitBudget)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	ackPath := filepath.Join(dir, "ack.json")
	for {
		if _, err := os.Stat(ackPath); err == nil {
			release, lerr := lockState(waitCtx, dir)
			if lerr == nil {
				var ack controlAck
				aerr := readJSON(ackPath, &ack)
				if aerr == nil && ack.ID == req.ID {
					_ = os.Remove(ackPath)
					st, serr := readState(dir)
					release()
					if serr != nil {
						return State{}, serr
					}
					if ack.Error != "" {
						return st, errors.New(ack.Error)
					}
					return st, nil
				}
				release()
			}
		}
		if !supervisorRunning(s.Home) {
			return State{}, errors.New("hevd service supervisor stopped")
		}
		select {
		case <-waitCtx.Done():
			return State{}, errors.New("timed out waiting for hevd service supervisor")
		case <-tick.C:
		}
	}
}
