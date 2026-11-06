package intake

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// servicePlan is pure so all platform definitions can be checked without installing jobs.
func servicePlan(platform, home, executable, config, action string) (string, string, []string, error) {
	quote := func(s string) string {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	escape := func(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
	switch platform {
	case "linux":
		path := filepath.Join(home, ".config", "systemd", "user", "hev-intake.service")
		body := "[Unit]\nDescription=HEV watch\n[Service]\nExecStart=" + quote(executable) + " watch linear run --config " + quote(config) + "\nRestart=no\n[Install]\nWantedBy=default.target\n"
		args := []string{"--user", action, "hev-intake.service"}
		if action == "install" {
			args = []string{"--user", "daemon-reload"}
		}
		return path, body, append([]string{"systemctl"}, args...), nil
	case "darwin":
		path := filepath.Join(home, "Library", "LaunchAgents", "com.hev.intake.plist")
		body := `<?xml version="1.0"?><plist version="1.0"><dict><key>Label</key><string>com.hev.intake</string><key>ProgramArguments</key><array>`
		for _, a := range []string{executable, "watch", "linear", "run", "--config", config} {
			body += "<string>" + escape(a) + "</string>"
		}
		body += `</array><key>RunAtLoad</key><false/><key>KeepAlive</key><false/></dict></plist>`
		domain := fmt.Sprintf("gui/%d", os.Getuid())
		args := []string{"launchctl", "print", domain + "/com.hev.intake"}
		switch action {
		case "install":
			args = []string{"launchctl", "bootstrap", domain, path}
		case "start":
			args = []string{"launchctl", "kickstart", domain + "/com.hev.intake"}
		case "stop":
			args = []string{"launchctl", "kill", "SIGTERM", domain + "/com.hev.intake"}
		}
		return path, body, args, nil
	case "windows":
		path := filepath.Join(home, ".hev", "intake-task.xml")
		body := `<?xml version="1.0" encoding="UTF-8"?><Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Triggers/><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><ExecutionTimeLimit>PT0S</ExecutionTimeLimit></Settings><Actions><Exec><Command>` + escape(executable) + `</Command><Arguments>watch linear run --config &quot;` + escape(config) + `&quot;</Arguments></Exec></Actions></Task>`
		args := []string{"schtasks", "/Query", "/TN", "hev-intake"}
		switch action {
		case "install":
			args = []string{"schtasks", "/Create", "/TN", "hev-intake", "/XML", path, "/F"}
		case "start":
			args = []string{"schtasks", "/Run", "/TN", "hev-intake"}
		case "stop":
			args = []string{"schtasks", "/End", "/TN", "hev-intake"}
		}
		return path, body, args, nil
	}
	return "", "", nil, fmt.Errorf("unsupported service platform %s", platform)
}

func runService(args []string) error {
	action := args[0]
	config := ""
	if len(args) == 3 && args[1] == "--config" {
		var err error
		config, err = filepath.Abs(args[2])
		if err != nil {
			return err
		}
	} else if len(args) != 1 {
		return fmt.Errorf("usage: hev watch %s [--config FILE]", action)
	}
	if action == "install" {
		if config == "" {
			return fmt.Errorf("--config FILE required")
		}
		if _, err := loadConfig(config); err != nil {
			return err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return applyService(runtime.GOOS, home, exe, config, action, func(command []string) error {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
}

func applyService(platform, home, exe, config, action string, execute func([]string) error) error {
	path, body, command, err := servicePlan(platform, home, exe, config, action)
	if err != nil {
		return err
	}
	if action == "install" {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err = os.WriteFile(path, []byte(body), 0600); err != nil {
			return err
		}
	}
	if platform == "darwin" && action == "install" {
		// An unloaded job also fails bootout; bootstrap remains the authoritative result.
		_ = execute([]string{"launchctl", "bootout", fmt.Sprintf("gui/%d/com.hev.intake", os.Getuid())})
	}
	return execute(command)
}
