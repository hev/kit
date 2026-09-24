package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Both labels can be overridden so that a second, isolated kit — a test
// sandbox on a machine that already runs the real jobs — never addresses them.
func launchdLabel() string { return envOr("HEV_LAUNCHD_LABEL", "com.hev.hevd") }

// serveLabel is the launchd job older kits ran the read side under. The
// dashboard is a container now; the label survives so that `up` and `down`
// can retire a job an older `up` installed.
func serveLabel() string { return envOr("HEV_SERVE_LAUNCHD_LABEL", "com.hev.serve") }

// launchdJob is one KeepAlive LaunchAgent running a hev subcommand.
type launchdJob struct {
	Label string
	Bin   string
	Args  []string
	Env   map[string]string
	// LogStem names the stdout/stderr files: <stem>.out and <stem>.err.
	LogStem string
}

func launchdDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func (j launchdJob) target() string { return launchdDomain() + "/" + j.Label }

func (j launchdJob) plistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", j.Label+".plist")
}

func (j launchdJob) bootstrapArgs(plistPath string) []string {
	return []string{"bootstrap", launchdDomain(), plistPath}
}

func (j launchdJob) bootoutArgs() []string { return []string{"bootout", j.target()} }

func (j launchdJob) loaded() bool {
	return exec.Command("launchctl", "print", j.target()).Run() == nil
}

// bootout unloads the job if it is loaded. Unloading is what stops a KeepAlive
// job; signalling its process only makes launchd start another.
func (j launchdJob) bootout() (wasLoaded bool, err error) {
	if !j.loaded() {
		return false, nil
	}
	if out, err := exec.Command("launchctl", j.bootoutArgs()...).CombinedOutput(); err != nil {
		return true, fmt.Errorf("launchctl bootout %s: %s: %w", j.Label, strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

// ensure writes the plist and loads the job. A job that is already loaded from
// an identical plist is left exactly as it is, so that re-running `hev up`
// does not restart a scan in progress.
func (j launchdJob) ensure(home string, restart bool) (changed bool, err error) {
	path := j.plistPath(home)
	want := []byte(j.plist())
	have, _ := os.ReadFile(path)
	if !restart && bytes.Equal(have, want) && j.loaded() {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		return false, fmt.Errorf("write plist: %w", err)
	}
	// Replace only our own job when reinstalling.
	if _, err := j.bootout(); err != nil {
		return false, err
	}
	if out, err := exec.Command("launchctl", j.bootstrapArgs(path)...).CombinedOutput(); err != nil {
		return false, fmt.Errorf("launchctl bootstrap %s: %s: %w", j.Label, strings.TrimSpace(string(out)), err)
	}
	return true, nil
}

func (j launchdJob) plist() string {
	var args, env strings.Builder
	for _, a := range append([]string{j.Bin}, j.Args...) {
		args.WriteString("        <string>" + plistEscape(a) + "</string>\n")
	}
	keys := make([]string, 0, len(j.Env))
	for k := range j.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env.WriteString("        <key>" + plistEscape(k) + "</key>\n        <string>" + plistEscape(j.Env[k]) + "</string>\n")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + plistEscape(j.Label) + `</string>

    <key>ProgramArguments</key>
    <array>
` + args.String() + `    </array>

    <key>EnvironmentVariables</key>
    <dict>
` + env.String() + `    </dict>

    <key>KeepAlive</key>
    <true/>

    <key>RunAtLoad</key>
    <true/>

    <key>StandardOutPath</key>
    <string>` + plistEscape(j.LogStem+".out") + `</string>

    <key>StandardErrorPath</key>
    <string>` + plistEscape(j.LogStem+".err") + `</string>

    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
`
}

const launchdPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

// jobEnv is what a job needs that launchd will not give it. HOME is set
// explicitly because every path kit derives — config, lock, index state, the
// harness transcript roots — hangs off it, and launchd supplies the login
// home no matter what the installing shell had.
func jobEnv(home, configPath string) map[string]string {
	env := map[string]string{"PATH": launchdPath, "HEV_CONFIG": configPath, "HOME": home}
	if v := os.Getenv("HEV_HOST"); v != "" {
		env["HEV_HOST"] = v
	}
	return env
}

func daemonJob(binPath, home, configPath string) launchdJob {
	return launchdJob{
		Label: launchdLabel(), Bin: binPath, Args: []string{"daemon"},
		Env: jobEnv(home, configPath), LogStem: filepath.Join(home, ".hev", "hevd"),
	}
}

// installSelf copies the running binary to ~/.local/bin/hev, the stable path
// the launchd jobs name, and returns it. An identical copy is left alone;
// updated says the path now holds a different binary than a running job has.
func installSelf(home string) (destBin string, updated bool, err error) {
	self, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf("resolve executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return "", false, fmt.Errorf("resolve symlinks: %w", err)
	}
	destBin = filepath.Join(home, ".local", "bin", "hev")
	if self == destBin || sameFile(self, destBin) {
		return destBin, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(destBin), 0o755); err != nil {
		return "", false, fmt.Errorf("create install dir: %w", err)
	}
	if err := copyFile(self, destBin); err != nil {
		return "", false, fmt.Errorf("copy binary: %w", err)
	}
	return destBin, true, nil
}

func sameFile(a, b string) bool {
	x, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b)
	return err == nil && bytes.Equal(x, y)
}
