// Package scheduler installs the hourly schedule that runs "agent-downlink run": a systemd
// user timer on Linux, a launchd agent on macOS. Both catch up one missed run after the
// machine was off or asleep. Neither runs while the user is logged out.
package scheduler

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Label is the launchd label and the plist's base name.
const Label = "com.github.jwp23.agent-downlink"

const (
	serviceName = "agent-downlink.service"
	timerName   = "agent-downlink.timer"
)

// SystemdService is the unit the timer starts.
func SystemdService(binary string) string {
	return "[Unit]\n" +
		"Description=agent-downlink: archive agent session records\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=" + systemdQuote(binary) + " run\n"
}

// systemdQuote quotes a single ExecStart= argument per systemd.syntax(7): ExecStart= splits
// its value into words the same way a shell would, so a binary path with whitespace needs
// quoting, and a literal double quote or backslash inside it needs escaping. A path with none
// of those characters is left as-is.
func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"\\") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// SystemdTimer fires hourly. Persistent=true runs once at login for runs missed while off.
func SystemdTimer() string {
	return "[Unit]\n" +
		"Description=Run agent-downlink hourly\n" +
		"\n" +
		"[Timer]\n" +
		"OnCalendar=hourly\n" +
		"Persistent=true\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=timers.target\n"
}

// LaunchdPlist fires at minute 0 of every hour. StartCalendarInterval, unlike StartInterval,
// runs once on wake for runs missed during sleep.
func LaunchdPlist(binary string) string {
	var escaped strings.Builder
	_ = xml.EscapeText(&escaped, []byte(binary)) // strings.Builder never returns a write error
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + escaped.String() + `</string>
		<string>run</string>
	</array>
	<key>StartCalendarInterval</key>
	<dict>
		<key>Minute</key>
		<integer>0</integer>
	</dict>
</dict>
</plist>
`
}

// Scheduler installs and removes the hourly schedule for one user.
type Scheduler struct {
	Home   string // the user's home directory
	Binary string // absolute path of agent-downlink; the schedule runs "<Binary> run"
	GOOS   string // "linux" or "darwin"
	UID    int    // launchd's domain is gui/<UID>
	Exec   func(name string, args ...string) error
}

// New is the scheduler for the current user on the current OS.
func New(home, binary string) *Scheduler {
	return &Scheduler{Home: home, Binary: binary, GOOS: runtime.GOOS, UID: os.Getuid(), Exec: run}
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Scheduler) unitDir() string { return filepath.Join(s.Home, ".config", "systemd", "user") }
func (s *Scheduler) plistPath() string {
	return filepath.Join(s.Home, "Library", "LaunchAgents", Label+".plist")
}
func (s *Scheduler) launchdService() string { return fmt.Sprintf("gui/%d/%s", s.UID, Label) }

// Install writes the schedule and starts it. Installing over an earlier install replaces it.
func (s *Scheduler) Install() error {
	switch s.GOOS {
	case "linux":
		if err := writeFile(filepath.Join(s.unitDir(), serviceName), SystemdService(s.Binary)); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(s.unitDir(), timerName), SystemdTimer()); err != nil {
			return err
		}
		if err := s.Exec("systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
		return s.Exec("systemctl", "--user", "enable", "--now", timerName)
	case "darwin":
		if err := writeFile(s.plistPath(), LaunchdPlist(s.Binary)); err != nil {
			return err
		}
		// bootstrap refuses a label that is already loaded, so unload any earlier copy. When
		// none is loaded bootout fails, which is the normal first-install case.
		_ = s.Exec("launchctl", "bootout", s.launchdService())
		return s.Exec("launchctl", "bootstrap", fmt.Sprintf("gui/%d", s.UID), s.plistPath())
	default:
		return s.unsupported()
	}
}

// Remove stops the schedule and removes its files. Removing what is not installed succeeds.
func (s *Scheduler) Remove() error {
	switch s.GOOS {
	case "linux":
		installed := exists(filepath.Join(s.unitDir(), timerName))
		if installed {
			if err := s.Exec("systemctl", "--user", "disable", "--now", timerName); err != nil {
				return err
			}
		}
		for _, name := range []string{timerName, serviceName} {
			if err := removeIfPresent(filepath.Join(s.unitDir(), name)); err != nil {
				return err
			}
		}
		if installed {
			return s.Exec("systemctl", "--user", "daemon-reload")
		}
		return nil
	case "darwin":
		_ = s.Exec("launchctl", "bootout", s.launchdService()) // fails when not loaded; nothing to do then
		return removeIfPresent(s.plistPath())
	default:
		return s.unsupported()
	}
}

func (s *Scheduler) unsupported() error {
	return fmt.Errorf("no scheduler support for %s; run 'agent-downlink run' hourly by other means", s.GOOS)
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
