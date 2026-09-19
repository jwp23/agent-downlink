package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jwp23/agent-downlink/internal/scheduler"
)

func cmdTimer(e env, args []string) int {
	if len(args) != 1 || (args[0] != "install" && args[0] != "remove") {
		_, _ = fmt.Fprintln(e.stderr, "usage: agent-downlink timer install|remove")
		return 2
	}
	binary, err := ownBinary()
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink timer: %v\n", err)
		return 1
	}
	s := scheduler.New(e.home, binary)
	if args[0] == "install" {
		err = s.Install()
	} else {
		err = s.Remove()
	}
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink timer %s: %v\n", args[0], err)
		return 1
	}
	if args[0] == "install" {
		_, _ = fmt.Fprintln(e.stdout, "Hourly schedule installed. It runs while you are logged in and catches up one missed run.")
	} else {
		_, _ = fmt.Fprintln(e.stdout, "Hourly schedule removed.")
	}
	return 0
}

// ownBinary is the path the schedule will run: this executable, with symlinks resolved so the
// schedule survives a relinked shim.
func ownBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
