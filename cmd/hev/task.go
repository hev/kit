package main

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode/utf16"
)

const windowsTaskName = `hev\hevd`

// renderTaskXML is independent of the host OS. Task Scheduler executes the
// binary directly (no shell expansion of paths or credentials).
func renderTaskXML(binary, home, userSID, config string) string {
	escape := func(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
	arguments := "daemon"
	// HEV_CONFIG is captured in arguments, since scheduled tasks need not inherit
	// the environment of the shell which installed them.
	if config != "" {
		arguments += " --config " + windowsQuoteArg(config)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.3" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
 <RegistrationInfo><Description>hev transcript capture</Description><URI>\hev\hevd</URI></RegistrationInfo>
 <Triggers><BootTrigger><Repetition><Interval>PT1M</Interval><StopAtDurationEnd>false</StopAtDurationEnd></Repetition><Enabled>true</Enabled></BootTrigger><LogonTrigger><Repetition><Interval>PT1M</Interval><StopAtDurationEnd>false</StopAtDurationEnd></Repetition><Enabled>true</Enabled><UserId>%s</UserId></LogonTrigger></Triggers>
 <Principals><Principal id="User"><UserId>%s</UserId><LogonType>S4U</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
 <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>true</StartWhenAvailable><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><Enabled>true</Enabled><Hidden>true</Hidden><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><RestartOnFailure><Interval>PT1M</Interval><Count>999</Count></RestartOnFailure></Settings>
 <Actions Context="User"><Exec><Command>%s</Command><Arguments>%s</Arguments><WorkingDirectory>%s</WorkingDirectory></Exec></Actions>
</Task>
`, escape(userSID), escape(userSID), escape(binary), escape(arguments), escape(home))
}

// Windows CommandLineToArgvW quoting, including trailing backslashes.
func windowsQuoteArg(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		if r == '\\' {
			slashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
		} else {
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, slashes*2))
	b.WriteByte('"')
	return b.String()
}

// Task Scheduler expects the declared encoding to match the file bytes.
func taskXMLBytes(text string) []byte {
	text = strings.Replace(text, `encoding="UTF-8"`, `encoding="UTF-16"`, 1)
	units := utf16.Encode([]rune(text))
	out := make([]byte, 2+2*len(units))
	out[0], out[1] = 0xff, 0xfe
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2+i*2:], u)
	}
	return out
}
