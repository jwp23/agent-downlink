// Command agent-downlink archives AI-agent session records to client-side-encrypted
// object storage and keeps a decrypted local mirror of every machine's records.
package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const usage = `Usage: agent-downlink <command>

Commands:
  setup [--no-timer]     Configure this machine and install the hourly timer
  add-machine <name>     Make another machine's records readable here
  run                    Full cycle: copy local records, push, pull (what the timer runs)
  push                   Copy local records into the mirror and push them
  pull [--transfers=N]   Pull every other readable machine into the mirror
  status                 Show the age of each step's last success and any current error
  timer install|remove   Install or remove the hourly schedule
  help                   Show this text
`

// env is everything a command needs from the process, so tests substitute all of it.
type env struct {
	home        string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool // a person is watching stderr; false under the timer
}

// command runs one subcommand and returns the process exit status.
type command func(e env, args []string) int

var commands = map[string]command{
	"setup":       cmdSetup,
	"add-machine": cmdAddMachine,
	"run":         cmdRun,
	"push":        cmdPush,
	"pull":        cmdPull,
	"status":      cmdStatus,
	"timer":       cmdTimer,
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent-downlink: cannot find the home directory: %v\n", err)
		os.Exit(1)
	}
	e := env{
		home:        home,
		stdin:       os.Stdin,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		interactive: term.IsTerminal(int(os.Stderr.Fd())),
	}
	os.Exit(dispatch(e, os.Args[1:]))
}

func dispatch(e env, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(e.stdout, usage)
		return 0
	}
	cmd, ok := commands[args[0]]
	if !ok {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	return cmd(e, args[1:])
}
